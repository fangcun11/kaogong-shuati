package api

import (
	"crypto/rand"
	"encoding/hex"
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

type subSpec struct {
	Name   string
	Weight int
}

type moduleSpec struct {
	Name  string
	Count int
	Tint  string
	Subs  []subSpec
}

type paperSource struct {
	ID         string
	Label      string
	Categories []string
	Like       string
}

type paperPick struct {
	QuestionID int64
	GroupID    string
	GroupIndex int
	GroupTotal int
	Module     string
	SubIdx     int
}

var xingceModules = []moduleSpec{
	{"政治理论", 20, "tint-red", []subSpec{{"新思想", 10}, {"时事政治", 6}, {"马克思主义", 2}, {"毛中特", 2}}},
	{"常识判断", 15, "tint-amber", []subSpec{{"人文常识", 6}, {"法律常识", 3}, {"科技常识", 3}, {"地理国情", 2}, {"经济常识", 1}}},
	{"言语理解与表达", 30, "tint-blue", []subSpec{{"片段阅读", 12}, {"逻辑填空", 10}, {"语句表达", 8}}},
	{"数量关系", 10, "tint-orange", []subSpec{{"数学运算", 10}}},
	{"判断推理", 35, "tint-violet", []subSpec{{"图形推理", 9}, {"定义判断", 9}, {"类比推理", 9}, {"逻辑判断", 8}}},
	{"资料分析", 20, "tint-green", []subSpec{{"综合", 7}, {"增长", 6}, {"比重", 3}, {"平均数", 3}, {"倍数", 1}}},
}

var xingcePaperSources = []paperSource{
	{ID: "tikuA", Label: "国考", Like: "国考"},
	{ID: "tikuB", Label: "联考省考", Categories: []string{
		"黑龙江", "吉林", "湖北", "新疆", "江西", "河南", "天津", "重庆", "福建", "安徽",
		"云南", "河北", "山西", "广西", "青海", "陕西", "贵州", "湖南", "辽宁", "甘肃",
		"海南", "内蒙古", "宁夏", "西藏", "四川",
	}},
	{ID: "tikuC", Label: "独立命题", Categories: []string{
		"江苏", "浙江", "广东", "山东", "北京", "上海", "深圳市考", "广州市考",
	}},
	{ID: "tikuD", Label: "选调/其他", Categories: []string{"选调", "政法干警"}},
}

var zhiCePaperSources = []paperSource{
	{ID: "zcA", Label: "联考A类", Categories: []string{"联考A类"}},
	{ID: "zcB", Label: "联考B类", Categories: []string{"联考B类"}},
	{ID: "zcC", Label: "联考C类", Categories: []string{"联考C类"}},
	{ID: "sd", Label: "山东", Categories: []string{"山东"}},
}

type zhiCeTemplate struct {
	Count   int
	Modules []moduleSpec
}

var zhiCeTemplates = map[string]zhiCeTemplate{
	"联考A类": {Count: 100, Modules: []moduleSpec{
		{"常识判断", 20, "tint-amber", []subSpec{{"人文常识", 8}, {"科技常识", 4}, {"法律常识", 3}, {"地理国情", 3}, {"经济常识", 2}}},
		{"言语理解与表达", 20, "tint-blue", []subSpec{{"逻辑填空", 8}, {"片段阅读", 8}, {"语句表达", 4}}},
		{"数量关系", 10, "tint-orange", []subSpec{{"数学运算", 10}}},
		{"判断推理", 35, "tint-violet", []subSpec{{"图形推理", 7}, {"定义判断", 9}, {"类比推理", 9}, {"逻辑判断", 10}}},
		{"资料分析", 15, "tint-green", []subSpec{{"综合", 6}, {"增长", 4}, {"比重", 2}, {"平均数", 2}, {"倍数", 1}}},
	}},
	// B/C 类的官方模块包含“数量分析/综合分析”，题库分类目前没有这两个
	// 独立分类，先按通用五模块抽取，避免生成空卷。
	"联考B类": {Count: 100, Modules: []moduleSpec{
		{"常识判断", 20, "tint-amber", []subSpec{{"人文常识", 8}, {"科技常识", 4}, {"法律常识", 3}, {"地理国情", 3}, {"经济常识", 2}}},
		{"言语理解与表达", 25, "tint-blue", []subSpec{{"逻辑填空", 10}, {"片段阅读", 10}, {"语句表达", 5}}},
		{"数量关系", 15, "tint-orange", []subSpec{{"数学运算", 15}}},
		{"判断推理", 30, "tint-violet", []subSpec{{"图形推理", 6}, {"定义判断", 8}, {"类比推理", 8}, {"逻辑判断", 8}}},
		{"资料分析", 10, "tint-green", []subSpec{{"综合", 4}, {"增长", 3}, {"比重", 1}, {"平均数", 1}, {"倍数", 1}}},
	}},
	"联考C类": {Count: 100, Modules: []moduleSpec{
		{"常识判断", 20, "tint-amber", []subSpec{{"人文常识", 8}, {"科技常识", 4}, {"法律常识", 3}, {"地理国情", 3}, {"经济常识", 2}}},
		{"言语理解与表达", 25, "tint-blue", []subSpec{{"逻辑填空", 10}, {"片段阅读", 10}, {"语句表达", 5}}},
		{"数量关系", 10, "tint-orange", []subSpec{{"数学运算", 10}}},
		{"判断推理", 30, "tint-violet", []subSpec{{"图形推理", 6}, {"定义判断", 8}, {"类比推理", 8}, {"逻辑判断", 8}}},
		{"资料分析", 15, "tint-green", []subSpec{{"综合", 6}, {"增长", 4}, {"比重", 2}, {"平均数", 2}, {"倍数", 1}}},
	}},
	"山东": {Count: 90, Modules: []moduleSpec{
		{"常识判断", 15, "tint-amber", []subSpec{{"人文常识", 6}, {"科技常识", 3}, {"法律常识", 3}, {"地理国情", 2}, {"经济常识", 1}}},
		{"言语理解与表达", 20, "tint-blue", []subSpec{{"逻辑填空", 8}, {"片段阅读", 8}, {"语句表达", 4}}},
		{"数量关系", 5, "tint-orange", []subSpec{{"数学运算", 5}}},
		{"判断推理", 35, "tint-violet", []subSpec{{"图形推理", 7}, {"定义判断", 9}, {"类比推理", 9}, {"逻辑判断", 10}}},
		{"资料分析", 15, "tint-green", []subSpec{{"综合", 6}, {"增长", 4}, {"比重", 2}, {"平均数", 2}, {"倍数", 1}}},
	}},
}

func zhiCeTemplateFor(category string) (zhiCeTemplate, bool) {
	t, ok := zhiCeTemplates[category]
	if !ok {
		t = zhiCeTemplates["联考A类"]
	}
	return t, ok
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
	notice := ""
	if req.Subject == "事业编·职测" {
		if req.Category == "" {
			req.Category = "联考A类"
		}
		tpl, exact := zhiCeTemplateFor(req.Category)
		modules = tpl.Modules
		defaultCount = tpl.Count
		duration = 90
		if !exact {
			notice = "职测卷型未知，已按联考 A 类模板组卷"
		} else if req.Category == "联考B类" || req.Category == "联考C类" {
			notice = "该卷型暂按通用五模块抽题，官方特殊模块会在后续版本补齐"
		}
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

	sources := xingcePaperSources
	mainSource := ""
	if req.Subject == "事业编·职测" {
		sources = zhiCePaperSources
		mainSource = sourceOfCategory(req.Category)
	}
	weak := map[string]bool{}
	for _, name := range req.Weak {
		weak[name] = true
	}
	picks, buildNotices, err := s.buildPaperPicks(r, paperBuildOptions{
		Subject: req.Subject, Modules: modules, Sources: sources, MainSource: mainSource,
		Count: req.Count, DiffMin: diffMin, DiffMax: diffMax, Weak: weak,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "组卷抽题失败")
		return
	}
	picks = trimPaperPicks(picks, req.Count)
	if len(picks) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "题库中没有符合条件的题目"})
		return
	}
	ids := make([]int64, 0, len(picks))
	for _, p := range picks {
		ids = append(ids, p.QuestionID)
	}
	questions, err := s.fetchQuestionsByIDs(r, ids)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "读取组卷题目失败")
		return
	}
	questions = s.enrichMaterials(r, req.Subject, questions)
	byID := make(map[int64]questionJSON, len(questions))
	for _, q := range questions {
		byID[q.QuestionID] = q
	}
	ordered := make([]questionJSON, 0, len(picks))
	for _, p := range picks {
		q, ok := byID[p.QuestionID]
		if !ok {
			continue
		}
		q.Module = p.Module
		q.GroupID = p.GroupID
		q.GroupIndex = p.GroupIndex
		q.GroupTotal = p.GroupTotal
		ordered = append(ordered, q)
	}
	questions = ordered
	sourceCounts := s.sourceCounts(r, ids)
	failedSources := make([]string, 0)
	checkSources := sources
	if mainSource != "" {
		for _, src := range sources {
			if src.ID == mainSource {
				checkSources = []paperSource{src}
				break
			}
		}
	}
	for _, src := range checkSources {
		if sourceCounts[src.ID] == 0 {
			failedSources = append(failedSources, src.ID)
			buildNotices = append(buildNotices, "「"+src.Label+"」无可用题目")
		}
	}
	title := fmt.Sprintf("智能组卷 · %s %d题", req.Subject, len(questions))
	if req.Subject == "事业编·职测" {
		title = fmt.Sprintf("智能组卷 · %s·%s %d题", req.Subject, req.Category, len(questions))
		duration = 90
	} else {
		duration = (len(questions)*120 + 65) / 130
		if duration < 5 {
			duration = 5
		}
	}
	if len(questions) < req.Count {
		buildNotices = append(buildNotices, fmt.Sprintf("题库资源有限，实际组卷 %d 题", len(questions)))
	}
	if len(buildNotices) > 0 {
		notice = strings.Join(buildNotices, "；")
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "paperId": newPaperID(), "title": title, "subject": req.Subject,
		"category": req.Category, "total": len(questions), "durationMinutes": duration,
		"difficulty":   map[string]int{"min": diffMin, "max": diffMax},
		"sourceCounts": sourceCounts, "failedSources": failedSources,
		"questions": questions, "notice": notice,
	})
}

func (s *Server) sourceCounts(r *http.Request, ids []int64) map[string]int {
	counts := map[string]int{
		"tikuA": 0, "tikuB": 0, "tikuC": 0, "tikuD": 0,
		"zcA": 0, "zcB": 0, "zcC": 0, "sd": 0, "other": 0,
	}
	if len(ids) == 0 {
		return counts
	}
	marks := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := s.db.QueryContext(r.Context(), `
		SELECT q.questionId, p.category FROM questions q
		JOIN papers p ON p.id = q.paperId
		WHERE q.questionId IN (`+marks+`) GROUP BY q.questionId`, args...)
	if err != nil {
		return counts
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var category string
		if rows.Scan(&id, &category) == nil {
			counts[sourceOfCategory(category)]++
		}
	}
	return counts
}

func sourceOfCategory(category string) string {
	switch category {
	case "联考A类":
		return "zcA"
	case "联考B类":
		return "zcB"
	case "联考C类":
		return "zcC"
	case "山东":
		return "sd"
	}
	if strings.Contains(category, "国考") {
		return "tikuA"
	}
	liankao := map[string]bool{
		"黑龙江": true, "吉林": true, "湖北": true, "新疆": true, "江西": true,
		"河南": true, "天津": true, "重庆": true, "福建": true, "安徽": true,
		"云南": true, "河北": true, "山西": true, "广西": true, "青海": true,
		"陕西": true, "贵州": true, "湖南": true, "辽宁": true, "甘肃": true,
		"海南": true, "内蒙古": true, "宁夏": true, "西藏": true, "四川": true,
	}
	if liankao[category] {
		return "tikuB"
	}
	duli := map[string]bool{
		"江苏": true, "浙江": true, "广东": true, "山东": true, "北京": true,
		"上海": true, "深圳市考": true, "广州市考": true,
	}
	if duli[category] {
		return "tikuC"
	}
	if category == "选调" || category == "政法干警" {
		return "tikuD"
	}
	return "other"
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
