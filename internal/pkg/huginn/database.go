package huginn

import (
	"embed"
	"sync"

	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// p0f.fp 为上游基础签名库（可经 make p0f-update 刷新），p0f.local.fp 为本项目
// 本地定制签名（overlay）；两者拼接后加载，与上游 ffi 的 include_str 拼接行为一致。
//
//go:embed p0f.fp
//go:embed p0f.local.fp
var fpFS embed.FS

// sigPos 索引项：定位一条签名在 entries 中的位置。
type sigPos struct {
	labelIdx int
	sigIdx   int
}

// tcpIndexKey TCP 索引键：ip 版本 + 选项布局串 + 载荷分类。
// 对应 huginn-net-db 的 TcpIndexKey。
type tcpIndexKey struct {
	ipVersionKey ipVersion
	olayoutKey   string
	pclassKey    payloadSize
}

// httpIndexKey HTTP 索引键：HTTP 版本。
// 对应 huginn-net-db 的 HttpIndexKey。
type httpIndexKey struct {
	httpVersionKey httpVersion
}

// tcpCollection TCP 签名集合（条目 + 索引），用于请求 / 响应两个方向。
// 对应 huginn-net-db 的 FingerprintCollection<TcpObservation, ...>。
type tcpCollection struct {
	entries []labeledTCP
	index   map[tcpIndexKey][]sigPos
}

// httpCollection HTTP 签名集合（条目 + 索引），用于请求 / 响应两个方向。
// 对应 huginn-net-db 的 FingerprintCollection<HttpObservation, ...>。
type httpCollection struct {
	entries []labeledHTTP
	index   map[httpIndexKey][]sigPos
}

// tcpOlayoutKey 将选项布局渲染为索引键串（逗号连接）。
func tcpOlayoutKey(olayout []tcpOption) string {
	var sb []byte
	for i, opt := range olayout {
		if i > 0 {
			sb = append(sb, ',')
		}
		sb = append(sb, opt.String()...)
	}
	return string(sb)
}

// tcpIndexKeysForSignature 为一条 TCP 签名生成全部索引键
// （对应 generate_index_keys_for_db_entry；Any 展开为 V4+V6 / Zero+NonZero 笛卡尔积）。
func tcpIndexKeysForSignature(sig *tcpSignature) []tcpIndexKey {
	versions := []ipVersion{sig.version}
	if sig.version == ipAny {
		versions = []ipVersion{ipV4, ipV6}
	}
	pclasses := []payloadSize{sig.pclass}
	if sig.pclass == payloadAny {
		pclasses = []payloadSize{payloadZero, payloadNonZero}
	}
	olayoutKey := tcpOlayoutKey(sig.olayout)
	keys := make([]tcpIndexKey, 0, len(versions)*len(pclasses))
	for _, v := range versions {
		for _, pc := range pclasses {
			keys = append(keys, tcpIndexKey{ipVersionKey: v, olayoutKey: olayoutKey, pclassKey: pc})
		}
	}
	return keys
}

// newTCPCollection 构建 TCP 签名集合并生成索引（对应 FingerprintCollection::new）。
func newTCPCollection(entries []labeledTCP) *tcpCollection {
	c := &tcpCollection{entries: entries, index: make(map[tcpIndexKey][]sigPos)}
	for li := range entries {
		for si := range entries[li].sigs {
			for _, key := range tcpIndexKeysForSignature(&entries[li].sigs[si]) {
				c.index[key] = append(c.index[key], sigPos{labelIdx: li, sigIdx: si})
			}
		}
	}
	return c
}

// httpIndexKeysForSignature 为一条 HTTP 签名生成全部索引键
// （对应 generate_http_index_keys；Any 展开为 V10+V11）。
func httpIndexKeysForSignature(sig *httpSignature) []httpIndexKey {
	if sig.version == httpAny {
		return []httpIndexKey{{httpVersionKey: httpV10}, {httpVersionKey: httpV11}}
	}
	return []httpIndexKey{{httpVersionKey: sig.version}}
}

// newHTTPCollection 构建 HTTP 签名集合并生成索引（对应 FingerprintCollection::new）。
func newHTTPCollection(entries []labeledHTTP) *httpCollection {
	c := &httpCollection{entries: entries, index: make(map[httpIndexKey][]sigPos)}
	for li := range entries {
		for si := range entries[li].sigs {
			for _, key := range httpIndexKeysForSignature(&entries[li].sigs[si]) {
				c.index[key] = append(c.index[key], sigPos{labelIdx: li, sigIdx: si})
			}
		}
	}
	return c
}

// findBestMatch 按观测值索引键锁定候选集，遍历计算距离并返回距离最小的
// 标签、签名与质量分（对应 FingerprintCollection::find_best_match）。
// 无可识别结果时返回 (nil, nil, 0)。
func (c *tcpCollection) findBestMatch(observed *tcpObservation) (*label, *tcpSignature, float64) {
	key := tcpIndexKey{
		ipVersionKey: observed.version,
		olayoutKey:   tcpOlayoutKey(observed.olayout),
		pclassKey:    observed.pclass,
	}
	candidates, ok := c.index[key]
	if !ok || len(candidates) == 0 {
		return nil, nil, 0
	}

	var (
		bestLabel *label
		bestSig   *tcpSignature
		minDist   = uint32(^uint32(0))
	)
	for _, pos := range candidates {
		e := &c.entries[pos.labelIdx]
		sig := &e.sigs[pos.sigIdx]
		if d := calculateTCPDistance(observed, sig); d != nil && *d < minDist {
			minDist = *d
			bestLabel = &e.label
			bestSig = sig
		}
	}
	if bestSig == nil {
		return nil, nil, 0
	}
	return bestLabel, bestSig, tcpDistanceToScore(minDist)
}

// findBestMatch 按观测值索引键锁定候选集，遍历计算距离并返回距离最小的
// 标签、签名与质量分（对应 FingerprintCollection::find_best_match）。
// 无可识别结果时返回 (nil, nil, 0)。
func (c *httpCollection) findBestMatch(observed *httpObservation) (*label, *httpSignature, float64) {
	candidates, ok := c.index[httpIndexKey{httpVersionKey: observed.version}]
	if !ok || len(candidates) == 0 {
		return nil, nil, 0
	}

	var (
		bestLabel *label
		bestSig   *httpSignature
		minDist   = uint32(^uint32(0))
	)
	for _, pos := range candidates {
		e := &c.entries[pos.labelIdx]
		sig := &e.sigs[pos.sigIdx]
		if d := calculateHTTPDistance(observed, sig); d != nil && *d < minDist {
			minDist = *d
			bestLabel = &e.label
			bestSig = sig
		}
	}
	if bestSig == nil {
		return nil, nil, 0
	}
	return bestLabel, bestSig, httpDistanceToScore(minDist)
}

// database 组合 p0f 签名库：TCP 请求 / 响应与 HTTP 请求 / 响应四个方向集合。
// 对应 huginn-net-db 的 Database。
type database struct {
	tcpRequest   *tcpCollection
	tcpResponse  *tcpCollection
	httpRequest  *httpCollection
	httpResponse *httpCollection
}

// loadDatabase 解析拼接后的 p0f 指纹库文本并构建索引集合。
func loadDatabase(contents string) (*database, error) {
	sections, err := parseSections(contents)
	if err != nil {
		return nil, err
	}
	return newDatabase(sections), nil
}

// newDatabase 由中间解析结果构建数据库（对应 Database::from_str）。
func newDatabase(out *parsedSections) *database {
	return &database{
		tcpRequest:   newTCPCollection(out.tcpRequest),
		tcpResponse:  newTCPCollection(out.tcpResponse),
		httpRequest:  newHTTPCollection(out.httpRequest),
		httpResponse: newHTTPCollection(out.httpResponse),
	}
}

var (
	globalDBOnce sync.Once
	globalDB     *database
	globalDBErr  error
)

// globalDatabase 返回懒加载的全局签名库（p0f.fp 与 p0f.local.fp 拼接）。
// 解析失败后每次调用都返回同一错误（对应上游 OnceLock 的 Err 状态）。
func globalDatabase() (*database, error) {
	globalDBOnce.Do(func() {
		p0f, err := fpFS.ReadFile("p0f.fp")
		if err != nil {
			globalDBErr = errors.WithStack(err)
			return
		}
		local, err := fpFS.ReadFile("p0f.local.fp")
		if err != nil {
			globalDBErr = errors.WithStack(err)
			return
		}
		globalDB, globalDBErr = loadDatabase(string(p0f) + "\n" + string(local))
	})
	return globalDB, globalDBErr
}
