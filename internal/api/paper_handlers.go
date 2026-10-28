package api

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type paperGenerateRequest struct {
	Subject    string   `json:"subject"`
	Category   string   `json:"category"`
	Count      int      `json:"count"`
	Difficulty string   `json:"difficulty"`
	Weak       []string `json:"weak"`
}

type moduleSpec struct {
	Name  string
	Count int
	Tint  string
}

var xingceModules = []moduleSpec{
	{"政治理论", 20, "tint-red"},
	{"常识判断", 15, "tint-amber"},
	{"言语理解与表达", 30, "tint-blue"},
	{"数量关系", 10, "tint-orange"},
	{"判断推理", 35, "tint-violet"},
	{"资料分析", 20, "tint-green"},
}

var zhiCeModules = []moduleSpec{
	{"政治理论", 10, "tint-red"},
	{"常识判断", 15, "tint-amber"},
	{"言语理解与表达", 25, "tint-blue"},
	{"数量关系", 10, "tint-orange"},
	{"判断推理", 25, "tint-violet"},
	{"资料分析", 15, "tint-green"},
}

func (s *Server) generatePaper(w http.ResponseWriter, r *http.Request) {
	var req paperGenerateRequest
	if err := decodeBody(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "请求格式不正确")
		return
	}
	if req.Subject != "公务员·行测" && req.Subject != "事业编·职测" {
		writeError(w, http.StatusBadRequest, "BAD_SUBJECT", "当前支持公务员·行测、事业编·职测")
		return
	}
	modules := xingceModules
	defaultCount := 130
	duration := 120
	if req.Subject == "事业编·职测" {
		modules = zhiCeModules
		defaultCount = 100
		duration = 90
	}
	if req.Count <= 0 {
		req.Count = defaultCount
	}
	if req.Count > defaultCount {
		req.Count = defaultCount
	}
	if req.Category == "" {
		req.Category = "联考A类"
	}
	diffMin, diffMax := difficultyRange(req.Difficulty)
	from, to := yearRange()

	quotas := moduleQuotas(modules, req.Count)
	weak := map[string]bool{}
	for _, m := range req.Weak {
		weak[m] = true
	}
	// Weak modules get a modest boost; the remaining quota is reduced from
	// the largest other modules so the total remains stable.
	if len(weak) > 0 {
		for i := range quotas {
			if weak[modules[i].Name] {
				quotas[i] = quotas[i] * 130 / 100
			}
		}
		normalizeQuotas(quotas, req.Count, modules, weak)
	}

	picked := make([]int64, 0, req.Count)
	seen := map[int64]bool{}
	moduleOf := map[int64]string{}
	for i, m := range modules {
		n := quotas[i]
		if n <= 0 {
			continue
		}
		rows, err := s.db.QueryContext(r.Context(), `
			SELECT qc.question_id
			FROM question_categories qc
			JOIN questions q ON q.questionId = qc.question_id
			JOIN papers p ON p.id = q.paperId
			WHERE qc.subject = ? AND qc.category = ?
			  AND substr(p.name,1,4) BETWEEN ? AND ?
			  AND COALESCE(q.difficulty, 5) BETWEEN ? AND ?
			GROUP BY qc.question_id ORDER BY RANDOM() LIMIT ?`,
			req.Subject, m.Name, from, to, diffMin, diffMax, n)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "组卷抽题失败")
			return
		}
		for rows.Next() {
			var id int64
			if rows.Scan(&id) == nil && !seen[id] {
				seen[id] = true
				picked = append(picked, id)
				moduleOf[id] = m.Name
			}
		}
		rows.Close()
	}
	// Fill any shortfall from the whole subject.
	if len(picked) < req.Count {
		rows, err := s.db.QueryContext(r.Context(), `
			SELECT qc.question_id FROM question_categories qc
			JOIN questions q ON q.questionId = qc.question_id
			JOIN papers p ON p.id = q.paperId
			WHERE qc.subject = ? AND substr(p.name,1,4) BETWEEN ? AND ?
			  AND COALESCE(q.difficulty, 5) BETWEEN ? AND ?
			GROUP BY qc.question_id ORDER BY RANDOM() LIMIT ?`,
			req.Subject, from, to, diffMin, diffMax, req.Count-len(picked))
		if err == nil {
			for rows.Next() {
				var id int64
				if rows.Scan(&id) == nil && !seen[id] {
					seen[id] = true
					picked = append(picked, id)
					moduleOf[id] = "综合"
				}
			}
			rows.Close()
		}
	}
	if len(picked) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "题库中没有符合条件的题目"})
		return
	}
	questions, err := s.fetchQuestionsByIDs(r, picked)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "读取组卷题目失败")
		return
	}
	questions = s.enrichMaterials(r, req.Subject, questions)
	for i := range questions {
		questions[i].Module = moduleOf[questions[i].QuestionID]
	}
	title := fmt.Sprintf("智能组卷 · %s %d题", req.Subject, len(questions))
	notice := ""
	if len(questions) < req.Count {
		notice = fmt.Sprintf("题库资源有限，实际组卷 %d 题", len(questions))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "paperId": newPaperID(), "title": title, "subject": req.Subject,
		"category": req.Category, "total": len(questions), "durationMinutes": duration,
		"difficulty":   map[string]int{"min": diffMin, "max": diffMax},
		"sourceCounts": map[string]int{"all": len(questions)}, "failedSources": []string{},
		"questions": questions, "notice": notice,
	})
}

func (s *Server) fetchQuestionsByIDs(r *http.Request, ids []int64) ([]questionJSON, error) {
	if len(ids) == 0 {
		return []questionJSON{}, nil
	}
	marks := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := s.db.QueryContext(r.Context(), `
		SELECT questionId, paperId, chapter, type, content, contentHtml, options, answer, answerIndex, difficulty, analysis
		FROM questions WHERE questionId IN (`+marks+`) GROUP BY questionId`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byID := map[int64]questionJSON{}
	for rows.Next() {
		q, err := scanQuestion(rows)
		if err == nil {
			byID[q.QuestionID] = q
		}
	}
	out := make([]questionJSON, 0, len(ids))
	for _, id := range ids {
		if q, ok := byID[id]; ok {
			out = append(out, q)
		}
	}
	return out, rows.Err()
}

func moduleQuotas(modules []moduleSpec, total int) []int {
	sum := 0
	for _, m := range modules {
		sum += m.Count
	}
	raw := make([]float64, len(modules))
	for i, m := range modules {
		raw[i] = float64(m.Count) * float64(total) / float64(sum)
	}
	out := make([]int, len(modules))
	remain := total
	for i, v := range raw {
		out[i] = int(v)
		remain -= out[i]
	}
	for remain > 0 {
		best := 0
		for i := range raw {
			if raw[i]-float64(out[i]) > raw[best]-float64(out[best]) {
				best = i
			}
		}
		out[best]++
		remain--
	}
	return out
}

func normalizeQuotas(quotas []int, total int, modules []moduleSpec, weak map[string]bool) {
	for {
		sum := 0
		for _, n := range quotas {
			sum += n
		}
		if sum == total {
			return
		}
		if sum > total {
			best := -1
			for i := range quotas {
				if !weak[modules[i].Name] && (best < 0 || quotas[i] > quotas[best]) {
					best = i
				}
			}
			if best < 0 || quotas[best] == 0 {
				for i := range quotas {
					if quotas[i] > 0 {
						quotas[i]--
						break
					}
				}
			} else {
				quotas[best]--
			}
		} else {
			best := 0
			for i := range quotas {
				if quotas[i] > quotas[best] {
					best = i
				}
			}
			quotas[best]++
		}
	}
}

func difficultyRange(d string) (int, int) {
	switch d {
	case "easy":
		return 1, 3
	case "hard":
		return 5, 9
	case "random":
		return 1, 9
	default:
		return 3, 6
	}
}

func newPaperID() string {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("paper_gen_%d", time.Now().UnixNano())
	}
	return "paper_gen_" + hex.EncodeToString(b)
}

var _ = sql.ErrNoRows
var _ = json.Valid
