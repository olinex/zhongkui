// docsnap 在源码树内烘焙 RAG 知识快照到 internal/docs/ragdata/snapshot.json。
//
// 发布二进制不携带源码树与 Go 工具链，zki sync docs / sync schema 无法在运行期渲染包注释
// 与数据表知识，因此由本工具在构建期生成快照，经 go:embed 随二进制发布；运行期在源码树
// 不可用时回退到该快照。CI 用 -check 拦截快照漂移。
//
// 用法：
//
//	make docs              # 生成文档与 RAG 快照
//	scripts/check.sh docs  # 只校验、不写盘
package main

import (
	// 标准库
	"bytes"
	"flag"
	"fmt"
	"os"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/docs"
)

func main() {
	root := flag.String("root", ".", "项目根目录")
	check := flag.Bool("check", false, "只校验快照是否与源码一致，不写盘")
	flag.Parse()

	if err := run(*root, *check); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run 执行快照生成或校验。
func run(root string, check bool) error {
	snapshot, err := docs.BuildRAGSnapshot(root)
	if err != nil {
		return err
	}
	data, err := docs.MarshalRAGSnapshot(snapshot)
	if err != nil {
		return err
	}
	path := docs.RAGSnapshotPath(root)
	if check {
		existing, readErr := os.ReadFile(path)
		if readErr != nil {
			return fmt.Errorf("failed to read RAG snapshot (%s): %w", path, readErr)
		}
		if !bytes.Equal(existing, data) {
			return fmt.Errorf("RAG snapshot is outdated (%s), run make docs to regenerate", path)
		}
		fmt.Printf("RAG 快照与源码一致：叙述页 %d 篇、数据表 %d 张\n", len(snapshot.Narratives), len(snapshot.Schemas))
		return nil
	}
	if err := docs.WriteRAGSnapshotFile(root); err != nil {
		return err
	}
	fmt.Printf("已生成 RAG 快照 %s：叙述页 %d 篇、数据表 %d 张\n", path, len(snapshot.Narratives), len(snapshot.Schemas))
	return nil
}
