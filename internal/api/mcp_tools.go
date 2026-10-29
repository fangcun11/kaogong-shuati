package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"kaogong/internal/mcp"
)

// The MCP tools deliberately call the existing HTTP handlers in-process. That
// keeps one implementation of group trimming, material enrichment, ownership
// checks and custom-question summaries; the alternative (a second SQL layer)
// drifts as soon as either side changes.

func (s *Server) mcpTools() []mcp.Tool {
	return []mcp.Tool{
		{
			Name: "study_overview",
			Description: "学习概览：各科目题量、已做数、累计正确率、错题数和最近 7 天练习量。" +
				"用来回答“我学得怎么样”“哪科该多练”。",
			InputSchema: mcpObject(map[string]any{
				"subject": mcpString("可选，限定某个科目（如 公务员·行测）"),
			}),
			Handler: s.mcpStudyOverview,
		},
		{
			Name: "list_questions",
			Description: "列出用户的错题、收藏或未做过的题。返回题目摘要，不含答案。" +
				"需要完整题干和解析时再用 get_question。",
			InputSchema: mcpObject(map[string]any{
				"state":   mcpEnum("题目状态", "wrong", "favorite", "unseen"),
				"subject": mcpString("可选，按科目过滤"),
				"limit":   mcpInteger("返回条数，默认 20", 1, 100),
				"offset":  mcpInteger("分页偏移，默认 0", 0, 100000),
			}),
			Handler: s.mcpListQuestions,
		},
		{
			Name: "get_question",
			Description: "读取单题：题干、选项、材料、图片、答案与解析。支持内置题数字 id 和自定义题 custom-<id>。" +
				"默认包含答案，includeAnswer=false 可只取题干。",
			InputSchema: mcpObject(map[string]any{
				"id":            map[string]any{"type": []string{"string", "integer"}, "description": "题目 id，如 123456 或 custom-12"},
				"includeAnswer": mcpBoolean("是否返回答案与解析，默认 true"),
			}, "id"),
			Handler: s.mcpGetQuestion,
		},
		{
			Name:        "weakness_report",
			Description: "薄弱章节报告：按章节聚合练习次数与正确率，按正确率从低到高排序，用于制定练习计划。",
			InputSchema: mcpObject(map[string]any{
				"subject":     mcpString("可选，限定科目"),
				"minAttempts": mcpInteger("纳入统计的最少练习次数，默认 3", 1, 1000),
				"limit":       mcpInteger("返回章节数，默认 10", 1, 50),
			}),
			Handler: s.mcpWeaknessReport,
		},
		{
			Name: "compose_practice",
			Description: "按科目/章节/难度组一组练习题，返回题干与选项但不含答案（避免提前剧透），也不写入练习记录。" +
				"要讲解时对每题调用 get_question。",
			InputSchema: mcpObject(map[string]any{
				"subject":    mcpString("科目全名，如 公务员·行测、公务员·申论、事业编·综应"),
				"n":          mcpInteger("题目数量，默认 10", 1, 50),
				"group":      mcpString("可选，知识点分组（如 资料分析）"),
				"chapters":   mcpStringArray("可选，章节名列表，与 group 二选一"),
				"difficulty": mcpEnum("难度", "easy", "medium", "hard", "random"),
				"mock":       mcpBoolean("是否只取模拟卷"),
			}, "subject"),
			Handler: s.mcpComposePractice,
		},
	}
}

// ---- tool implementations -------------------------------------------------

func (s *Server) mcpStudyOverview(ctx context.Context, raw json.RawMessage) (*mcp.Result, error) {
	var in struct {
		Subject string `json:"subject"`
	}
	if err := decodeToolArgs(raw, &in); err != nil {
		return nil, err
	}
	body, status, err := s.mcpGET(ctx, "/api/subjects", nil, s.subjects)
	if err != nil {
		return nil, err
	}
	var subjects []subjectRow
	if fail := decodeToolResponse(body, status, &subjects); fail != nil {
		return fail, nil
	}
	query := url.Values{}
	if subject := strings.TrimSpace(in.Subject); subject != "" {
		query.Set("subject", subject)
	}
	body, status, err = s.mcpGET(ctx, "/api/records/stats", query, s.stats)
	if err != nil {
		return nil, err
	}
	var stats statsResponse
	if fail := decodeToolResponse(body, status, &stats); fail != nil {
		return fail, nil
	}
	recent := make([]map[string]any, 0, len(stats.Last7))
	for _, day := range stats.Last7 {
		recent = append(recent, map[string]any{
			"date": day.Day, "attempts": day.Count, "correct": day.OK,
		})
	}
	out := map[string]any{
		"subjects":        subjects,
		"attempts":        stats.Total,
		"correct":         stats.Correct,
		"wrong":           stats.Wrong,
		"accuracyPercent": stats.Rate,
		"recent7Days":     recent,
	}
	if in.Subject != "" {
		out["subject"] = in.Subject
	}
	return mcp.JSONResult(out)
}

func (s *Server) mcpListQuestions(ctx context.Context, raw json.RawMessage) (*mcp.Result, error) {
	var in struct {
		State   string `json:"state"`
		Subject string `json:"subject"`
		Limit   int    `json:"limit"`
		Offset  int    `json:"offset"`
	}
	if err := decodeToolArgs(raw, &in); err != nil {
		return nil, err
	}
	state := strings.ToLower(strings.TrimSpace(in.State))
	if state == "" {
		state = "wrong"
	}
	limit := clampInt(in.Limit, 20, 1, 100)
	offset := in.Offset
	if offset < 0 {
		offset = 0
	}

	out := map[string]any{"state": state, "limit": limit, "offset": offset}
	switch state {
	case "wrong", "favorite":
		query := url.Values{}
		query.Set("limit", strconv.Itoa(limit))
		query.Set("offset", strconv.Itoa(offset))
		if subject := strings.TrimSpace(in.Subject); subject != "" {
			query.Set("subject", subject)
		}
		path, handler := "/api/records/wrong", http.HandlerFunc(s.wrongList)
		if state == "favorite" {
			path, handler = "/api/favorites", http.HandlerFunc(s.favorites)
		}
		body, status, err := s.mcpGET(ctx, path, query, handler)
		if err != nil {
			return nil, err
		}
		var page mcpQuestionPage
		if fail := decodeToolResponse(body, status, &page); fail != nil {
			return fail, nil
		}
		items := make([]mcpListItem, 0, len(page.List))
		for _, row := range page.List {
			items = append(items, mcpListItem{
				ID:         row.ID,
				QuestionID: row.QuestionID,
				Subject:    row.Subject,
				Chapter:    row.Chapter,
				Type:       row.Type,
				Preview:    truncateRunes(htmlToText(row.Content), 120),
				Time:       row.Time,
			})
		}
		out["items"] = items
		out["total"] = page.Total
		out["hasMore"] = page.HasMore
	case "unseen":
		user, ok := userFromContext(ctx)
		if !ok {
			return mcp.ErrorResult("缺少用户上下文"), nil
		}
		items, hasMore, err := s.mcpUnseen(ctx, user.ID, strings.TrimSpace(in.Subject), limit, offset)
		if err != nil {
			return nil, err
		}
		out["items"] = items
		out["hasMore"] = hasMore
	default:
		return mcp.ErrorResult("state 只支持 wrong、favorite、unseen"), nil
	}
	return mcp.JSONResult(out)
}

func (s *Server) mcpGetQuestion(ctx context.Context, raw json.RawMessage) (*mcp.Result, error) {
	var in struct {
		ID            mcpIDArg `json:"id"`
		IncludeAnswer *bool    `json:"includeAnswer"`
	}
	if err := decodeToolArgs(raw, &in); err != nil {
		return nil, err
	}
	id := strings.TrimSpace(string(in.ID))
	if id == "" {
		return mcp.ErrorResult("缺少题目 id"), nil
	}
	query := url.Values{}
	query.Set("id", id)
	body, status, err := s.mcpGET(ctx, "/api/question", query, s.question)
	if err != nil {
		return nil, err
	}
	var payload mcpQuestionPayload
	if fail := decodeToolResponse(body, status, &payload); fail != nil {
		return fail, nil
	}

	includeAnswer := true
	if in.IncludeAnswer != nil {
		includeAnswer = *in.IncludeAnswer
	}
	subject := strings.TrimSpace(payload.Subject)
	if subject == "" {
		subject = s.subjectForPaper(ctx, payload.PaperID)
	}
	stem := htmlToText(payload.Content)
	if stem == "" {
		stem = htmlToText(payload.ContentHTML)
	}
	material := htmlToText(payload.Material)
	if material == "" {
		material = htmlToText(payload.MaterialHTML)
	}
	out := map[string]any{
		"id":       payload.QuestionID,
		"subject":  subject,
		"chapter":  payload.Chapter,
		"type":     payload.Type,
		"stem":     stem,
		"options":  payload.Options,
		"material": material,
		"images":   extractQuestionImages(ctx, payload.ContentHTML, payload.MaterialHTML, payload.Content, payload.Material, payload.Analysis),
	}
	if includeAnswer {
		out["answer"] = payload.Answer
		out["analysis"] = htmlToText(payload.Analysis)
	}
	return mcp.JSONResult(out)
}

func (s *Server) mcpWeaknessReport(ctx context.Context, raw json.RawMessage) (*mcp.Result, error) {
	var in struct {
		Subject     string `json:"subject"`
		MinAttempts int    `json:"minAttempts"`
		Limit       int    `json:"limit"`
	}
	if err := decodeToolArgs(raw, &in); err != nil {
		return nil, err
	}
	minAttempts := clampInt(in.MinAttempts, 3, 1, 1000)
	limit := clampInt(in.Limit, 10, 1, 50)
	query := url.Values{}
	if subject := strings.TrimSpace(in.Subject); subject != "" {
		query.Set("subject", subject)
	}
	body, status, err := s.mcpGET(ctx, "/api/records/stats", query, s.stats)
	if err != nil {
		return nil, err
	}
	var stats statsResponse
	if fail := decodeToolResponse(body, status, &stats); fail != nil {
		return fail, nil
	}
	items := make([]mcpWeakChapter, 0, len(stats.ByChapter))
	for _, chapter := range stats.ByChapter {
		if chapter.Count < int64(minAttempts) || strings.TrimSpace(chapter.Chapter) == "" {
			continue
		}
		items = append(items, mcpWeakChapter{
			Chapter:         chapter.Chapter,
			Attempts:        chapter.Count,
			Correct:         chapter.OK,
			AccuracyPercent: chapter.OK * 100 / chapter.Count,
		})
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].AccuracyPercent != items[j].AccuracyPercent {
			return items[i].AccuracyPercent < items[j].AccuracyPercent
		}
		return items[i].Attempts > items[j].Attempts
	})
	if len(items) > limit {
		items = items[:limit]
	}
	out := map[string]any{
		"subject":         in.Subject,
		"minAttempts":     minAttempts,
		"attempts":        stats.Total,
		"correct":         stats.Correct,
		"accuracyPercent": stats.Rate,
		"items":           items,
	}
	return mcp.JSONResult(out)
}

func (s *Server) mcpComposePractice(ctx context.Context, raw json.RawMessage) (*mcp.Result, error) {
	var in struct {
		Subject    string   `json:"subject"`
		N          int      `json:"n"`
		Group      string   `json:"group"`
		Chapters   []string `json:"chapters"`
		Difficulty string   `json:"difficulty"`
		Mock       bool     `json:"mock"`
	}
	if err := decodeToolArgs(raw, &in); err != nil {
		return nil, err
	}
	subject := strings.TrimSpace(in.Subject)
	if subject == "" {
		return mcp.ErrorResult("缺少 subject"), nil
	}
	n := clampInt(in.N, 10, 1, 50)
	query := url.Values{}
	query.Set("subject", subject)
	query.Set("n", strconv.Itoa(n))
	if group := strings.TrimSpace(in.Group); group != "" {
		query.Set("group", group)
	}
	if len(in.Chapters) > 0 {
		chapters := make([]string, 0, len(in.Chapters))
		for _, chapter := range in.Chapters {
			if chapter = strings.TrimSpace(chapter); chapter != "" {
				chapters = append(chapters, chapter)
			}
		}
		if len(chapters) > 0 {
			query.Set("chapters", strings.Join(chapters, ","))
		}
	}
	switch in.Difficulty {
	case "easy", "medium", "hard", "random":
		query.Set("difficulty", in.Difficulty)
	}
	if in.Mock {
		query.Set("mock", "1")
	}
	body, status, err := s.mcpGET(ctx, "/api/practice", query, s.practice)
	if err != nil {
		return nil, err
	}
	var questions []questionJSON
	if fail := decodeToolResponse(body, status, &questions); fail != nil {
		return fail, nil
	}
	items := make([]mcpPracticeQuestion, 0, len(questions))
	for _, q := range questions {
		stem := htmlToText(q.Content)
		if stem == "" {
			stem = htmlToText(q.ContentHTML)
		}
		items = append(items, mcpPracticeQuestion{
			ID:         q.QuestionID,
			Subject:    subject,
			Chapter:    q.Chapter,
			Type:       q.Type,
			Difficulty: q.Difficulty,
			Stem:       stem,
			Options:    q.Options,
			Images:     extractQuestionImages(ctx, q.ContentHTML, q.Content),
			GroupID:    q.GroupID,
			GroupIndex: q.GroupIndex,
			GroupTotal: q.GroupTotal,
		})
	}
	return mcp.JSONResult(map[string]any{
		"subject":        subject,
		"requested":      n,
		"count":          len(items),
		"answersOmitted": true,
		"questions":      items,
	})
}

// ---- in-process handler adapter ------------------------------------------

type mcpRecorder struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func newMCPRecorder() *mcpRecorder {
	return &mcpRecorder{header: make(http.Header)}
}

func (m *mcpRecorder) Header() http.Header { return m.header }

func (m *mcpRecorder) WriteHeader(status int) {
	if m.status == 0 {
		m.status = status
	}
}

func (m *mcpRecorder) Write(p []byte) (int, error) {
	if m.status == 0 {
		m.status = http.StatusOK
	}
	return m.body.Write(p)
}

// mcpGET invokes a read handler with the authenticated user already present in
// ctx, and returns the raw JSON response.
func (s *Server) mcpGET(ctx context.Context, path string, query url.Values, handler http.HandlerFunc) ([]byte, int, error) {
	if len(query) > 0 {
		path += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, 0, err
	}
	rec := newMCPRecorder()
	handler(rec, req)
	status := rec.status
	if status == 0 {
		status = http.StatusOK
	}
	return rec.body.Bytes(), status, nil
}

func decodeToolResponse(body []byte, status int, v any) *mcp.Result {
	if status < 200 || status >= 300 {
		var e errorBody
		message := strings.TrimSpace(string(body))
		if json.Unmarshal(body, &e) == nil && e.Error != "" {
			message = e.Error
		}
		if message == "" {
			message = fmt.Sprintf("HTTP %d", status)
		}
		return mcp.ErrorResult("%s", message)
	}
	if err := json.Unmarshal(body, v); err != nil {
		return mcp.ErrorResult("解析接口返回失败: %v", err)
	}
	return nil
}

func decodeToolArgs(raw json.RawMessage, v any) error {
	if len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, v)
}

// ---- shaping --------------------------------------------------------------

type mcpIDArg string

func (a *mcpIDArg) UnmarshalJSON(raw []byte) error {
	if string(raw) == "null" {
		*a = ""
		return nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		*a = mcpIDArg(strings.TrimSpace(s))
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(raw, &n); err == nil {
		*a = mcpIDArg(n.String())
		return nil
	}
	return fmt.Errorf("id 需要字符串或数字")
}

type mcpListRow struct {
	ID         any    `json:"id"`
	QuestionID any    `json:"questionId"`
	Subject    string `json:"subject"`
	Chapter    string `json:"chapter"`
	Time       string `json:"time"`
	Content    string `json:"content"`
	Type       any    `json:"type"`
}

type mcpQuestionPage struct {
	List    []mcpListRow `json:"list"`
	Total   int64        `json:"total"`
	Offset  int          `json:"offset"`
	Limit   int          `json:"limit"`
	HasMore bool         `json:"hasMore"`
}

type mcpListItem struct {
	ID         any    `json:"id,omitempty"`
	QuestionID any    `json:"questionId"`
	Subject    string `json:"subject,omitempty"`
	Chapter    string `json:"chapter,omitempty"`
	Type       any    `json:"type,omitempty"`
	Preview    string `json:"preview"`
	Time       string `json:"time,omitempty"`
}

type mcpQuestionPayload struct {
	QuestionID   any      `json:"questionId"`
	PaperID      int64    `json:"paperId"`
	Chapter      string   `json:"chapter"`
	Type         any      `json:"type"`
	Content      string   `json:"content"`
	ContentHTML  string   `json:"contentHtml"`
	Options      []string `json:"options"`
	Answer       string   `json:"answer"`
	Analysis     string   `json:"analysis"`
	Material     string   `json:"material"`
	MaterialHTML string   `json:"materialHtml"`
	Subject      string   `json:"subject"`
}

type mcpWeakChapter struct {
	Chapter         string `json:"chapter"`
	Attempts        int64  `json:"attempts"`
	Correct         int64  `json:"correct"`
	AccuracyPercent int64  `json:"accuracyPercent"`
}

type mcpPracticeQuestion struct {
	ID         int64    `json:"id"`
	Subject    string   `json:"subject"`
	Chapter    string   `json:"chapter,omitempty"`
	Type       int64    `json:"type"`
	Difficulty int64    `json:"difficulty,omitempty"`
	Stem       string   `json:"stem"`
	Options    []string `json:"options,omitempty"`
	Images     []string `json:"images,omitempty"`
	GroupID    string   `json:"groupId,omitempty"`
	GroupIndex int      `json:"groupIndex,omitempty"`
	GroupTotal int      `json:"groupTotal,omitempty"`
}

func (s *Server) mcpUnseen(ctx context.Context, userID int64, subject string, limit, offset int) ([]mcpListItem, bool, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT q.questionId, q.chapter, q.type, q.content, p.subjectName
		FROM questions q
		JOIN papers p ON p.id = q.paperId
		WHERE q.id = (SELECT MIN(q2.id) FROM questions q2 WHERE q2.questionId = q.questionId)
		  AND (? = '' OR p.subjectName = ?)
		  AND NOT EXISTS (
			SELECT 1 FROM practice_records r
			WHERE r.user_id = ? AND r.question_id = CAST(q.questionId AS TEXT)
		  )
		ORDER BY q.questionId
		LIMIT ? OFFSET ?`, subject, subject, userID, limit+1, offset)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	items := make([]mcpListItem, 0, limit+1)
	for rows.Next() {
		var id int64
		var chapter, content, subjectName string
		var qtype int64
		if err := rows.Scan(&id, &chapter, &qtype, &content, &subjectName); err != nil {
			return nil, false, err
		}
		items = append(items, mcpListItem{
			QuestionID: id,
			Subject:    subjectName,
			Chapter:    chapter,
			Type:       qtype,
			Preview:    truncateRunes(htmlToText(content), 120),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	return items, hasMore, nil
}

func (s *Server) subjectForPaper(ctx context.Context, paperID int64) string {
	if paperID == 0 {
		return ""
	}
	var subject string
	_ = s.db.QueryRowContext(ctx, `SELECT subjectName FROM papers WHERE id = ?`, paperID).Scan(&subject)
	return subject
}

// ---- schemas and text helpers --------------------------------------------

func mcpObject(props map[string]any, required ...string) map[string]any {
	out := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		out["required"] = required
	}
	return out
}

func mcpString(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

func mcpEnum(description string, values ...string) map[string]any {
	return map[string]any{"type": "string", "description": description, "enum": values}
}

func mcpInteger(description string, min, max int) map[string]any {
	return map[string]any{"type": "integer", "description": description, "minimum": min, "maximum": max}
}

func mcpBoolean(description string) map[string]any {
	return map[string]any{"type": "boolean", "description": description}
}

func mcpStringArray(description string) map[string]any {
	return map[string]any{
		"type": "array", "description": description,
		"items": map[string]any{"type": "string"},
	}
}

func clampInt(v, fallback, min, max int) int {
	if v == 0 {
		return fallback
	}
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

var (
	mcpBreakRE = regexp.MustCompile(`(?i)<\s*(?:br\s*/?|/(?:p|div|li|tr|h[1-6]|blockquote))\s*>`)
	mcpTagRE   = regexp.MustCompile(`(?s)<[^>]*>`)
	mcpImgRE   = regexp.MustCompile(`(?is)<img[^>]*\bsrc\s*=\s*["']([^"']+)["']`)
	mcpSpaceRE = regexp.MustCompile(`[ \t]+`)
)

func htmlToText(raw string) string {
	if raw == "" {
		return ""
	}
	text := strings.ReplaceAll(raw, "\r\n", "\n")
	text = mcpBreakRE.ReplaceAllString(text, "\n")
	text = mcpTagRE.ReplaceAllString(text, "")
	text = html.UnescapeString(text)
	text = strings.ReplaceAll(text, "\u00a0", " ")
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	blank := false
	for _, line := range lines {
		line = strings.TrimSpace(mcpSpaceRE.ReplaceAllString(line, " "))
		if line == "" {
			if blank || len(out) == 0 {
				continue
			}
			blank = true
			out = append(out, "")
			continue
		}
		blank = false
		out = append(out, line)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func extractQuestionImages(ctx context.Context, parts ...string) []string {
	base := mcpBaseURL(ctx)
	seen := map[string]bool{}
	out := make([]string, 0)
	for _, part := range parts {
		for _, match := range mcpImgRE.FindAllStringSubmatch(part, -1) {
			src := absoluteImageURL(base, strings.TrimSpace(match[1]))
			if src == "" || seen[src] {
				continue
			}
			seen[src] = true
			out = append(out, src)
		}
	}
	return out
}

func mcpBaseURL(ctx context.Context) string {
	base, _ := ctx.Value(mcpBaseURLKey{}).(string)
	return base
}

func absoluteImageURL(base, src string) string {
	lower := strings.ToLower(src)
	switch {
	case src == "":
		return ""
	case strings.HasPrefix(lower, "data:"),
		strings.HasPrefix(lower, "http://"),
		strings.HasPrefix(lower, "https://"):
		return src
	case strings.HasPrefix(src, "//"):
		return "https:" + src
	}
	if base == "" {
		return src
	}
	if !strings.HasPrefix(src, "/") {
		src = "/" + src
	}
	return strings.TrimRight(base, "/") + src
}
