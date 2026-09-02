// pipeline.go は parse → 検証 → レイアウト → 構図検証 → SVG → HTML の共通パイプライン。
package cli

import (
	"github.com/ZONO33LHD/archy-go/internal/diag"
	"github.com/ZONO33LHD/archy-go/internal/ir"
	"github.com/ZONO33LHD/archy-go/internal/render"
	"github.com/ZONO33LHD/archy-go/internal/render/architecture"
	"github.com/ZONO33LHD/archy-go/internal/validate"
)

// buildResult はパイプラインの結果。
type buildResult struct {
	Doc     *ir.Document
	Diags   diag.List
	Checks  []validate.CheckResult
	Profile string
	HTML    string
	// OK は品質プロファイル基準で合格したか。
	// standard: エラー 0 / showcase: エラー 0 かつ警告 0。
	OK bool
}

// build はパイプラインを実行する。qualityOverride は CLI フラグ (メタより優先)。
// runAllChecks が真なら profile にかかわらず全チェックを実行する (deliver 用)。
func build(diagramType string, data []byte, qualityOverride string, runAllChecks bool) *buildResult {
	res := &buildResult{Profile: qualityOverride}

	doc, ds := ir.Parse(diagramType, data)
	res.Diags = ds
	if doc == nil {
		if res.Profile == "" {
			res.Profile = "standard"
		}
		return res
	}
	res.Doc = doc
	if res.Profile == "" {
		res.Profile = doc.QualityProfile()
	}

	res.Diags = append(res.Diags, validate.Document(doc)...)
	if res.Diags.HasErrors() {
		return res
	}

	var renderer render.Renderer = architecture.New()
	layout, layoutDiags := renderer.Layout(doc)
	compDiags, checks := validate.Composition(layout, layoutDiags, res.Profile, runAllChecks)
	res.Diags = append(res.Diags, compDiags...)
	res.Checks = checks

	errs, warns := res.Diags.Count()
	res.OK = errs == 0 && (res.Profile != "showcase" || warns == 0)
	if !res.OK {
		return res
	}

	svg := renderer.SVG(layout)
	html, err := render.AssembleHTML(doc, svg)
	if err != nil {
		res.Diags = append(res.Diags, diag.Error("render/assemble", "", err.Error()))
		res.OK = false
		return res
	}
	res.HTML = html
	return res
}
