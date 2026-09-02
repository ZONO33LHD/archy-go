// Package validate はスキーマの先にある検証層を実装する。
//
//   - 参照整合性 (from/to/wraps/focus の実在、id の一意性)
//   - 文字列サニタイズ検査 (internal/safe の禁止文字)
//   - meta.output の出力パス制約 (§10.4。値は検証のみで、書き込み先には使わない)
//
// 構図検証 (composition checks) は composition.go を参照。
package validate

import (
	"fmt"

	"github.com/ZONO33LHD/archy-go/internal/diag"
	"github.com/ZONO33LHD/archy-go/internal/ir"
	"github.com/ZONO33LHD/archy-go/internal/outpath"
	"github.com/ZONO33LHD/archy-go/internal/safe"
)

// Document は参照整合性とサニタイズの検査を実行する。
func Document(doc *ir.Document) diag.List {
	var ds diag.List
	checkReferences(doc, &ds)
	checkStrings(doc, &ds)
	checkMetaOutput(doc, &ds)
	return ds
}

// checkReferences は id の一意性と参照の実在を検査する。
func checkReferences(doc *ir.Document, ds *diag.List) {
	compIDs := make(map[string]int, len(doc.Components))
	for i, c := range doc.Components {
		if prev, dup := compIDs[c.ID]; dup {
			d := diag.Error("ref/duplicate-id", fmt.Sprintf("/components/%d/id", i),
				fmt.Sprintf("component.id %s が /components/%d と重複しています", safe.ForDiag(c.ID), prev),
				"一方の id を一意な値に変更する")
			d.Subject = &diag.Subject{Surface: "component", ID: c.ID}
			diag.Append(ds, d)
			continue
		}
		compIDs[c.ID] = i
	}

	connIDs := make(map[string]int, len(doc.Connections))
	for i, c := range doc.Connections {
		ptr := fmt.Sprintf("/connections/%d", i)
		if prev, dup := connIDs[c.ID]; dup {
			d := diag.Error("ref/duplicate-id", ptr+"/id",
				fmt.Sprintf("connection.id %s が /connections/%d と重複しています", safe.ForDiag(c.ID), prev),
				"一方の id を一意な値に変更する")
			d.Subject = &diag.Subject{Surface: "connection", ID: c.ID}
			diag.Append(ds, d)
		} else {
			connIDs[c.ID] = i
		}
		for _, ref := range []struct{ field, id string }{{"from", c.From}, {"to", c.To}} {
			if _, ok := compIDs[ref.id]; !ok {
				d := diag.Error("ref/unknown-component", ptr+"/"+ref.field,
					fmt.Sprintf("%s が参照する component %s が存在しません", ref.field, safe.ForDiag(ref.id)),
					"実在する component.id を参照する", "components に対象を追加する")
				d.Subject = &diag.Subject{Surface: "connection", ID: c.ID}
				diag.Append(ds, d)
			}
		}
		if c.From == c.To {
			d := diag.Error("ref/self-loop", ptr,
				"自己参照の接続 (from == to) は architecture 図種ではサポートされません",
				"接続を削除するか、別のコンポーネントへ向ける")
			d.Subject = &diag.Subject{Surface: "connection", ID: c.ID}
			diag.Append(ds, d)
		}
		// via と channelX/channelY は経路の指定方法が競合する。同時指定を拒否して
		// 「片方が黙って無視される」曖昧さを排除する (via 優先で channel が捨てられる)。
		if len(c.Via) > 0 && (c.ChannelX != nil || c.ChannelY != nil) {
			d := diag.Error("ref/route-conflict", ptr,
				"via と channelX/channelY は同時に指定できません (経路指定が競合します)",
				"via のみ、または channelX/channelY のみを指定する")
			d.Subject = &diag.Subject{Surface: "connection", ID: c.ID}
			diag.Append(ds, d)
		}
	}

	for i, b := range doc.Boundaries {
		seen := map[string]bool{}
		for j, id := range b.Wraps {
			ptr := fmt.Sprintf("/boundaries/%d/wraps/%d", i, j)
			if _, ok := compIDs[id]; !ok {
				diag.Append(ds, diag.Error("ref/unknown-component", ptr,
					fmt.Sprintf("wraps が参照する component %s が存在しません", safe.ForDiag(id)),
					"実在する component.id を参照する"))
			}
			if seen[id] {
				diag.Append(ds, diag.Error("ref/duplicate-wrap", ptr,
					fmt.Sprintf("wraps 内で component %s が重複しています", safe.ForDiag(id)),
					"重複した参照を削除する"))
			}
			seen[id] = true
		}
	}

	viewIDs := map[string]int{}
	for i, v := range doc.Meta.Views {
		ptr := fmt.Sprintf("/meta/views/%d", i)
		if prev, dup := viewIDs[v.ID]; dup {
			diag.Append(ds, diag.Error("ref/duplicate-id", ptr+"/id",
				fmt.Sprintf("view.id %s が /meta/views/%d と重複しています", safe.ForDiag(v.ID), prev),
				"一方の id を一意な値に変更する"))
		} else {
			viewIDs[v.ID] = i
		}
		for j, id := range v.Focus {
			if _, ok := compIDs[id]; !ok {
				diag.Append(ds, diag.Error("ref/unknown-component", fmt.Sprintf("%s/focus/%d", ptr, j),
					fmt.Sprintf("focus が参照する component %s が存在しません", safe.ForDiag(id)),
					"実在する component.id を参照する"))
			}
		}
	}
}

// checkStrings は IR 由来の全文字列に禁止文字が無いか検査する。
//
// 単一行フィールド (title / label / sublabel / tag) は CheckSingleLine で改行・タブも拒否する
// (SVG では折り返されず縦にはみ出すため)。複数行を許すフィールド (note / card items) は CheckText。
func checkStrings(doc *ir.Document, ds *diag.List) {
	emit := func(ptr string, vs []safe.Violation, subject *diag.Subject) {
		for _, v := range vs {
			if ds.AtCap() {
				return
			}
			msg := fmt.Sprintf("禁止文字 %s がコードポイント位置 %d に含まれています", v.Rune, v.Index)
			fix := "該当文字を削除するか、通常の文字に置き換える"
			if v.Code == "sanitize/multiline" {
				msg = fmt.Sprintf("単一行フィールドに改行/タブ (%s) が位置 %d に含まれています", v.Rune, v.Index)
				fix = "改行・タブを削除する (このフィールドは 1 行のみ)"
			}
			d := diag.Error(v.Code, ptr, msg, fix)
			d.Subject = subject
			diag.Append(ds, d)
		}
	}
	line := func(ptr, s string, subject *diag.Subject) { emit(ptr, safe.CheckSingleLine(s), subject) }
	text := func(ptr, s string, subject *diag.Subject) { emit(ptr, safe.CheckText(s), subject) }

	line("/meta/title", doc.Meta.Title, &diag.Subject{Surface: "meta"})
	for i, v := range doc.Meta.Views {
		p := fmt.Sprintf("/meta/views/%d", i)
		s := &diag.Subject{Surface: "view", ID: v.ID}
		line(p+"/label", v.Label, s)
		text(p+"/note", v.Note, s)
	}
	for i, c := range doc.Components {
		p := fmt.Sprintf("/components/%d", i)
		s := &diag.Subject{Surface: "component", ID: c.ID}
		line(p+"/label", c.Label, s)
		line(p+"/sublabel", c.Sublabel, s)
		line(p+"/tag", c.Tag, s)
	}
	for i, b := range doc.Boundaries {
		line(fmt.Sprintf("/boundaries/%d/label", i), b.Label, &diag.Subject{Surface: "boundary"})
	}
	for i, c := range doc.Connections {
		s := &diag.Subject{Surface: "connection", ID: c.ID}
		line(fmt.Sprintf("/connections/%d/label", i), c.Label, s)
	}
	for i, c := range doc.Cards {
		p := fmt.Sprintf("/cards/%d", i)
		s := &diag.Subject{Surface: "card"}
		line(p+"/title", c.Title, s)
		for j, item := range c.Items {
			text(fmt.Sprintf("%s/items/%d", p, j), item, s)
		}
	}
}

// checkMetaOutput は meta.output の制約 (§10.4) を検査する。
// 値は検証のみで、実際の出力先は常に CLI 引数が決める (§10.0 の原則)。
func checkMetaOutput(doc *ir.Document, ds *diag.List) {
	if doc.Meta.Output == "" {
		return
	}
	if err := outpath.Validate(doc.Meta.Output); err != nil {
		diag.Append(ds, diag.Error("outpath/meta-output", "/meta/output",
			"meta.output が出力パス制約を満たしません: "+err.Error(),
			"cwd 配下の相対パスで拡張子 .html のパスに修正する"))
	}
}
