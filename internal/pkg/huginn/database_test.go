// 索引与匹配引擎单元测试：database.go。
//
// 覆盖索引键生成、TCP / HTTP 集合的最佳匹配查找与签名库加载错误路径。
package huginn

import "testing"

// TestTcpOlayoutKey 验证 TCP 选项布局键的生成。
func TestTcpOlayoutKey(t *testing.T) {
	olayout := []tcpOption{{kind: optMss}, {kind: optSok}, {kind: optEol, num: 1}}
	if got := tcpOlayoutKey(olayout); got != "mss,sok,eol+1" {
		t.Errorf("key = %q", got)
	}
	if got := tcpOlayoutKey(nil); got != "" {
		t.Errorf("empty key = %q", got)
	}
}

// TestTCPIndexKeysForSignature 验证 TCP 签名的索引键生成与通配展开。
func TestTCPIndexKeysForSignature(t *testing.T) {
	sig := &tcpSignature{version: ipV4, olayout: []tcpOption{{kind: optMss}}, pclass: payloadZero}
	keys := tcpIndexKeysForSignature(sig)
	if len(keys) != 1 {
		t.Fatalf("len = %d, want 1", len(keys))
	}
	if keys[0].ipVersionKey != ipV4 || keys[0].pclassKey != payloadZero {
		t.Errorf("key = %+v", keys[0])
	}

	// ipAny 展开为 V4 + V6
	sig.version = ipAny
	if keys := tcpIndexKeysForSignature(sig); len(keys) != 2 {
		t.Errorf("ipAny len = %d, want 2", len(keys))
	}

	// payloadAny 展开为 Zero + NonZero，与 ipAny 组合 → 4
	sig.pclass = payloadAny
	if keys := tcpIndexKeysForSignature(sig); len(keys) != 4 {
		t.Errorf("any combos len = %d, want 4", len(keys))
	}
}

// TestHTTPIndexKeysForSignature 验证 HTTP 签名的索引键生成与通配展开。
func TestHTTPIndexKeysForSignature(t *testing.T) {
	sig := &httpSignature{version: httpV11}
	keys := httpIndexKeysForSignature(sig)
	if len(keys) != 1 || keys[0].httpVersionKey != httpV11 {
		t.Errorf("keys = %+v", keys)
	}
	sig.version = httpAny
	if keys := httpIndexKeysForSignature(sig); len(keys) != 2 {
		t.Errorf("httpAny len = %d, want 2", len(keys))
	}
}

// TestTCPCollectionFindBestMatch 验证 TCP 集合的最佳匹配查找与各类不命中场景。
func TestTCPCollectionFindBestMatch(t *testing.T) {
	// 用 linuxSYNPacket 构造观测，索引与匹配全链路验证
	pkg, err := analyzeTCPIPv4(linuxSYNPacket())
	if err != nil {
		t.Fatalf("analyzeTCPIPv4: %v", err)
	}
	sig := linuxSignature()
	entries := []labeledTCP{
		{label: label{ty: labelSpecified, class: strptr("unix"), name: "Linux", flavor: strptr("3.1-3.10")}, sigs: []tcpSignature{sig}},
	}
	c := newTCPCollection(entries)
	lbl, gotSig, score := c.findBestMatch(pkg.tcpRequest)
	if lbl == nil {
		t.Fatal("findBestMatch returned nil label")
	}
	if lbl.name != "Linux" || lbl.flavor == nil || *lbl.flavor != "3.1-3.10" {
		t.Errorf("label = %+v", lbl)
	}
	if gotSig == nil {
		t.Error("findBestMatch returned nil signature")
	}
	if score != 1.0 {
		t.Errorf("score = %v, want 1.0", score)
	}

	// 载荷分类不同 → 索引键不命中
	bad := *pkg.tcpRequest
	bad.pclass = payloadNonZero
	if lbl, _, score := c.findBestMatch(&bad); lbl != nil || score != 0 {
		t.Errorf("pclass mismatch: lbl=%v score=%v, want nil/0", lbl, score)
	}

	// 怪癖不一致 → 候选存在但距离 nil → 无匹配
	bad = *pkg.tcpRequest
	bad.quirks = []quirk{quirkDf}
	if lbl, _, score := c.findBestMatch(&bad); lbl != nil || score != 0 {
		t.Errorf("quirks mismatch: lbl=%v score=%v, want nil/0", lbl, score)
	}

	// 空集合
	empty := newTCPCollection(nil)
	if lbl, _, score := empty.findBestMatch(pkg.tcpRequest); lbl != nil || score != 0 {
		t.Errorf("empty collection: lbl=%v score=%v", lbl, score)
	}
}

// TestHTTPCollectionFindBestMatch 验证 HTTP 集合的最佳匹配查找与版本不命中场景。
func TestHTTPCollectionFindBestMatch(t *testing.T) {
	obs := &httpObservation{
		version: httpV11,
		horder: []httpHeader{
			{name: "Server"},
			{name: "Date"},
			{name: "Content-Type"},
			{name: "Connection", value: strptr("keep-alive")},
		},
		habsent: nil,
		expsw:   "nginx/1.18.0",
	}
	sig := httpSignature{
		version: httpV11,
		horder: []httpHeader{
			{name: "Server"},
			{name: "Date"},
			{name: "Content-Type"},
			{optional: true, name: "Content-Length"},
			{name: "Connection", value: strptr("keep-alive")},
		},
		expsw: "nginx/",
	}
	entries := []labeledHTTP{
		{label: label{ty: labelSpecified, class: strptr("!"), name: "nginx", flavor: strptr("1.x")}, sigs: []httpSignature{sig}},
	}
	c := newHTTPCollection(entries)
	lbl, gotSig, score := c.findBestMatch(obs)
	if lbl == nil || lbl.name != "nginx" {
		t.Fatalf("label = %+v", lbl)
	}
	if gotSig == nil {
		t.Error("findBestMatch returned nil signature")
	}
	if score != 1.0 {
		t.Errorf("score = %v, want 1.0", score)
	}

	// 版本不命中 → 索引键缺失
	v10 := *obs
	v10.version = httpV10
	if lbl, _, score := c.findBestMatch(&v10); lbl != nil || score != 0 {
		t.Errorf("version miss: lbl=%v score=%v, want nil/0", lbl, score)
	}
}

// TestLoadDatabaseError 验证签名库加载对非法签名段报错、对空内容返回空库。
func TestLoadDatabaseError(t *testing.T) {
	// 非法签名段 → 解析失败
	_, err := loadDatabase("[tcp:request]\nlabel = s:unix:Bad:1.0\nsig = *:64:0\n")
	if err == nil {
		t.Error("loadDatabase with bad sig: want error")
	}
	// 空内容 → 无错误（空数据库）
	if _, err := loadDatabase(""); err != nil {
		t.Errorf("loadDatabase empty: %v", err)
	}
}
