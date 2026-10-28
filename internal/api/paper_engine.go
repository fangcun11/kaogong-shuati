package api

import (
	"math"
	"net/http"
	"strings"
)

type paperBuildOptions struct {
	Subject string
	Modules []moduleSpec
	Sources []paperSource
	Count   int
	DiffMin int
	DiffMax int
	Weak    map[string]bool
}

func (s *Server) buildPaperPicks(r *http.Request, opt paperBuildOptions) ([]paperPick, []string, error) {
	moduleQuotas := moduleQuotas(opt.Modules, opt.Count)
	if len(opt.Weak) > 0 {
		moduleQuotas = applyWeakQuotas(opt.Modules, moduleQuotas, opt.Weak, opt.Count)
	}
	seenQ := map[int64]bool{}
	seenG := map[string]bool{}
	notices := make([]string, 0)
	picks := make([]paperPick, 0, opt.Count)

	for mi, m := range opt.Modules {
		quota := moduleQuotas[mi]
		if quota <= 0 {
			continue
		}
		modulePicks := make([]paperPick, 0, quota)
		appendSingle := func(id int64, subIdx int) {
			if id == 0 || seenQ[id] {
				return
			}
			seenQ[id] = true
			modulePicks = append(modulePicks, paperPick{QuestionID: id, Module: m.Name, SubIdx: subIdx})
		}
		appendGroup := func(gid string, subIdx int) {
			members := s.materialGroupMembers(r, opt.Subject, gid)
			if len(members) == 0 {
				return
			}
			for i, id := range members {
				if seenQ[id] {
					continue
				}
				seenQ[id] = true
				modulePicks = append(modulePicks, paperPick{
					QuestionID: id, GroupID: gid, Module: m.Name, SubIdx: subIdx,
					GroupIndex: i, GroupTotal: len(members),
				})
			}
		}

		if m.Name == "资料分析" {
			groupCount := (quota + 4) / 5
			gids := s.pickGroupIDs(r, opt.Subject, m.Name, "", nil, opt.DiffMin, opt.DiffMax, 3, 6, groupCount, seenG)
			for _, gid := range gids {
				appendGroup(gid, 0)
			}
			if len(modulePicks) < quota {
				notices = append(notices, "资料分析材料组不足，已用单题补齐")
				for _, id := range s.pickSingleIDs(r, opt.Subject, m.Name, "", nil, 1, 9, quota-len(modulePicks), seenQ) {
					appendSingle(id, 0)
				}
			}
		} else {
			weights := make([]float64, len(opt.Sources))
			for i := range weights {
				weights[i] = 1
			}
			srcQuotas := allocateWeights(weights, quota)
			for si, src := range opt.Sources {
				srcQuota := srcQuotas[si]
				if srcQuota <= 0 {
					continue
				}
				subs := m.Subs
				if len(subs) == 0 {
					subs = []subSpec{{Name: "", Weight: 1}}
				}
				subWeights := make([]float64, len(subs))
				for i, sub := range subs {
					subWeights[i] = float64(sub.Weight)
				}
				subQuotas := allocateWeights(subWeights, srcQuota)
				for subIdx, sub := range subs {
					n := subQuotas[subIdx]
					if n <= 0 {
						continue
					}
					if m.Name == "判断推理" && sub.Name == "逻辑判断" && n >= 5 {
						before := len(modulePicks)
						gids := s.pickGroupIDs(r, opt.Subject, m.Name, sub.Name, &src, opt.DiffMin, opt.DiffMax, 4, 6, 1, seenG)
						for _, gid := range gids {
							appendGroup(gid, subIdx)
						}
						n -= minInt(n, len(modulePicks)-before)
					}
					if n <= 0 {
						continue
					}
					for _, id := range s.pickSingleIDs(r, opt.Subject, m.Name, sub.Name, &src, opt.DiffMin, opt.DiffMax, n, seenQ) {
						appendSingle(id, subIdx)
					}
				}
			}
			// Source pool can be uneven; fill from the whole subject, but never
			// split material groups (pickSingle excludes grouped questions).
			if len(modulePicks) < quota {
				need := quota - len(modulePicks)
				subs := m.Subs
				if len(subs) == 0 {
					subs = []subSpec{{Name: "", Weight: 1}}
				}
				weights := make([]float64, len(subs))
				for i, sub := range subs {
					weights[i] = float64(sub.Weight)
				}
				fillQuotas := allocateWeights(weights, need)
				for subIdx, sub := range subs {
					if fillQuotas[subIdx] <= 0 {
						continue
					}
					for _, id := range s.pickSingleIDs(r, opt.Subject, m.Name, sub.Name, nil, 1, 9, fillQuotas[subIdx], seenQ) {
						appendSingle(id, subIdx)
					}
				}
			}
		}

		// Keep the official module sequence and sub-type order. The group
		// members were appended together, so stable sorting keeps them adjacent.
		sortPicksStable(modulePicks)
		picks = append(picks, modulePicks...)
	}
	return picks, notices, nil
}

func applyWeakQuotas(modules []moduleSpec, quota []int, weak map[string]bool, total int) []int {
	out := append([]int(nil), quota...)
	for i := range out {
		if weak[modules[i].Name] {
			out[i] = int(math.Round(float64(out[i]) * 1.3))
		}
	}
	sum := 0
	for _, n := range out {
		sum += n
	}
	for guard := 0; sum != total && guard < 1000; guard++ {
		if sum > total {
			best := -1
			for i, n := range out {
				if !weak[modules[i].Name] && n > 0 && (best < 0 || n > out[best]) {
					best = i
				}
			}
			if best < 0 {
				for i, n := range out {
					if n > 0 && (best < 0 || n > out[best]) {
						best = i
					}
				}
			}
			if best < 0 {
				break
			}
			out[best]--
			sum--
		} else {
			best := -1
			for i, n := range out {
				if weak[modules[i].Name] && (best < 0 || n > out[best]) {
					best = i
				}
			}
			if best < 0 {
				for i, n := range out {
					if best < 0 || n > out[best] {
						best = i
					}
				}
			}
			if best < 0 {
				break
			}
			out[best]++
			sum++
		}
	}
	return out
}

func allocateWeights(weights []float64, total int) []int {
	out := make([]int, len(weights))
	if len(weights) == 0 || total <= 0 {
		return out
	}
	sum := 0.0
	for _, w := range weights {
		if w > 0 {
			sum += w
		}
	}
	if sum <= 0 {
		return out
	}
	remain := total
	for i, w := range weights {
		if w <= 0 {
			continue
		}
		out[i] = int(float64(total) * w / sum)
		remain -= out[i]
	}
	for remain > 0 {
		best := -1
		bestFrac := -1.0
		for i, w := range weights {
			if w <= 0 {
				continue
			}
			frac := float64(total)*w/sum - float64(out[i])
			if frac > bestFrac {
				bestFrac = frac
				best = i
			}
		}
		if best < 0 {
			break
		}
		out[best]++
		remain--
	}
	return out
}

func (s *Server) sourceWhere(src *paperSource) (string, []any) {
	if src == nil {
		return "1=1", nil
	}
	if src.Like != "" {
		return "p.category LIKE ?", []any{"%" + src.Like + "%"}
	}
	if len(src.Categories) > 0 {
		marks := strings.TrimSuffix(strings.Repeat("?,", len(src.Categories)), ",")
		args := make([]any, 0, len(src.Categories))
		for _, c := range src.Categories {
			args = append(args, c)
		}
		return "p.category IN (" + marks + ")", args
	}
	return "1=1", nil
}

func (s *Server) pickSingleIDs(r *http.Request, subject, module, sub string, src *paperSource, diffMin, diffMax, limit int, seen map[int64]bool) []int64 {
	if limit <= 0 {
		return nil
	}
	from, to := yearRange()
	where := []string{
		"qc.subject = ?", "qc.category = ?", "p.subjectName = ?",
		"substr(p.name,1,4) BETWEEN ? AND ?", "q.type = 1",
	}
	args := []any{subject, module, subject, from, to}
	if sub != "" {
		where = append(where, "qc.sub = ?")
		args = append(args, sub)
	}
	if diffMin == 1 && diffMax == 9 {
		where = append(where, "(q.difficulty IS NULL OR q.difficulty BETWEEN 1 AND 9)")
	} else {
		where = append(where, "q.difficulty BETWEEN ? AND ?")
		args = append(args, diffMin, diffMax)
	}
	srcSQL, srcArgs := s.sourceWhere(src)
	where = append(where, srcSQL, "NOT EXISTS (SELECT 1 FROM q_material_map mm WHERE mm.question_id = qc.question_id AND mm.subject = qc.subject AND mm.material_id IS NOT NULL)")
	args = append(args, srcArgs...)
	args = append(args, limit)
	query := `SELECT qc.question_id FROM question_categories qc
		JOIN questions q ON q.questionId = qc.question_id
		JOIN papers p ON p.id = q.paperId
		WHERE ` + strings.Join(where, " AND ") + ` ORDER BY RANDOM() LIMIT ?`
	rows, err := s.db.QueryContext(r.Context(), query, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := make([]int64, 0, limit)
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil && !seen[id] {
			out = append(out, id)
		}
	}
	return out
}

func (s *Server) pickGroupIDs(r *http.Request, subject, module, sub string, src *paperSource, diffMin, diffMax, minSize, maxSize, limit int, seen map[string]bool) []string {
	if limit <= 0 {
		return nil
	}
	from, to := yearRange()
	where := []string{
		"mm.subject = ?", "mm.material_id IS NOT NULL", "qc.category = ?",
		"p.subjectName = ?", "substr(p.name,1,4) BETWEEN ? AND ?", "q.type = 1",
	}
	args := []any{subject, module, subject, from, to}
	if sub != "" {
		where = append(where, "qc.sub = ?")
		args = append(args, sub)
	}
	if module == "资料分析" {
		where = append(where, "q.chapter LIKE '%资料%'")
	}
	if module == "判断推理" && sub == "逻辑判断" {
		where = append(where, "q.chapter LIKE '%逻辑%'")
	}
	if diffMin == 1 && diffMax == 9 {
		where = append(where, "(q.difficulty IS NULL OR q.difficulty BETWEEN 1 AND 9)")
	} else {
		where = append(where, "q.difficulty BETWEEN ? AND ?")
		args = append(args, diffMin, diffMax)
	}
	srcSQL, srcArgs := s.sourceWhere(src)
	where = append(where, srcSQL)
	args = append(args, srcArgs...)
	args = append(args, minSize, maxSize, limit)
	query := `SELECT mm.material_id FROM q_material_map mm
		JOIN questions q ON q.questionId = mm.question_id
		JOIN papers p ON p.id = q.paperId
		JOIN question_categories qc ON qc.question_id = mm.question_id AND qc.subject = mm.subject
		WHERE ` + strings.Join(where, " AND ") + `
		GROUP BY mm.material_id HAVING COUNT(*) BETWEEN ? AND ?
		ORDER BY RANDOM() LIMIT ?`
	rows, err := s.db.QueryContext(r.Context(), query, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := make([]string, 0, limit)
	for rows.Next() {
		var gid string
		if rows.Scan(&gid) == nil && !seen[gid] {
			seen[gid] = true
			out = append(out, gid)
		}
	}
	return out
}

func (s *Server) materialGroupMembers(r *http.Request, subject, gid string) []int64 {
	rows, err := s.db.QueryContext(r.Context(), `
		SELECT mm.question_id FROM q_material_map mm
		JOIN questions q ON q.questionId = mm.question_id
		WHERE mm.subject = ? AND mm.material_id = ?
		ORDER BY q.id`, subject, gid)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := make([]int64, 0)
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil {
			out = append(out, id)
		}
	}
	return out
}

func trimPaperPicks(picks []paperPick, count int) []paperPick {
	if len(picks) <= count {
		return picks
	}
	out := append([]paperPick(nil), picks...)
	over := len(out) - count
	// Prefer trimming lone questions from the tail, keeping material groups intact.
	for i := len(out) - 1; i >= 0 && over > 0; i-- {
		if out[i].GroupID != "" {
			continue
		}
		out = append(out[:i], out[i+1:]...)
		over--
	}
	// If still long, trim the last member of a group only.
	for i := len(out) - 1; i >= 0 && over > 0; i-- {
		if out[i].GroupID == "" {
			continue
		}
		if i < len(out)-1 && out[i+1].GroupID == out[i].GroupID {
			continue
		}
		out = append(out[:i], out[i+1:]...)
		over--
	}
	totals := map[string]int{}
	indexes := map[string]int{}
	for _, p := range out {
		if p.GroupID != "" {
			totals[p.GroupID]++
		}
	}
	for i := range out {
		if out[i].GroupID == "" {
			continue
		}
		out[i].GroupTotal = totals[out[i].GroupID]
		out[i].GroupIndex = indexes[out[i].GroupID]
		indexes[out[i].GroupID]++
	}
	return out
}

func sortPicksStable(picks []paperPick) {
	for i := 1; i < len(picks); i++ {
		for j := i; j > 0 && picks[j].SubIdx < picks[j-1].SubIdx; j-- {
			picks[j], picks[j-1] = picks[j-1], picks[j]
		}
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
