//go:build !profile
// +build !profile



package test

var Tracer = NewStageTracer()

type StageTracer struct{}

func NewStageTracer() *StageTracer       { return &StageTracer{} }
func (t *StageTracer) Begin(name string) {}
func (t *StageTracer) End()              {}
func (t *StageTracer) Report()           {}
