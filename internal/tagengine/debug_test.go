package tagengine

import (
	"context"
	"testing"

	"zhongkui.com/zhongkui/internal/models/tag"
)

func TestDryRunDebug(t *testing.T) {
	rules := []*tag.TagRule{
		{Name: "ua-curl", Rego: `package zki

tags contains "automation" if {
    contains(input.user_agent, "curl")
}`},
		{Name: "method-get", Rego: `package zki

tags contains "web_request" if {
    input.method == "GET"
}`},
	}
	for _, r := range rules {
		errs, ok := InspectRego(r.Rego)
		t.Logf("InspectRego name=%s ok=%v errs=%+v", r.Name, ok, errs)
	}
	group, err := CompileGroup(context.Background(), rules)
	t.Logf("compileRules err=%v group=%v", err, group)

	conflict := []*tag.TagRule{
		{Name: "r1", Rego: "package zki\n" + `tags := {"a"}`},
		{Name: "r2", Rego: "package zki\n" + `tags contains "b" if { input.method == "POST" }`},
	}
	errs, ok := InspectRego(conflict[0].Rego)
	t.Logf("conflict InspectRego r1 ok=%v errs=%+v", ok, errs)
	group2, err := CompileGroup(context.Background(), conflict)
	t.Logf("conflict compileRules err=%v group2=%v", err, group2)
	t.Logf("conflict extractRegoErrors=%+v", extractRegoErrors(err))
}
