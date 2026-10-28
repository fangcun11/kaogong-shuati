package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"kaogong/internal/ai"
)

func (s *Server) aiAgents(w http.ResponseWriter, r *http.Request) {
	list, err := s.ai.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "读取 AI 配置失败")
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) aiAgentByID(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "BAD_ID", "AI id 不合法")
		return
	}
	a, err := s.ai.Get(id)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "AI 不存在")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "读取 AI 配置失败")
		return
	}
	writeJSON(w, http.StatusOK, s.ai.View(*a))
}

func (s *Server) updateAIAgent(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "BAD_ID", "AI id 不合法")
		return
	}
	var fields map[string]any
	if err := decodeBody(w, r, &fields); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "请求格式不正确")
		return
	}
	if _, err := s.ai.Get(id); err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "AI 不存在")
		return
	}
	changed, err := s.ai.Update(id, fields)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "保存 AI 配置失败")
		return
	}
	a, _ := s.ai.Get(id)
	if a != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "promptChanged": changed, "agent": s.ai.View(*a)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) aiAgentHistory(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	items, err := s.ai.History(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "读取历史失败")
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) testAIAgent(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var req struct {
		Content string `json:"content"`
	}
	if err := decodeBody(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "请求格式不正确")
		return
	}
	if strings.TrimSpace(req.Content) == "" {
		req.Content = "请用一句话确认你可以正常工作。"
	}
	a, err := s.ai.Get(id)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "AI 不存在")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "读取 AI 配置失败")
		return
	}
	content, err := s.ai.Call(a, req.Content)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"content": content})
}

func (s *Server) clearExplainCache(w http.ResponseWriter, r *http.Request) {
	u, _ := userFromContext(r.Context())
	res, err := s.db.ExecContext(r.Context(), `DELETE FROM ai_explains WHERE user_id = ?`, u.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "清空缓存失败")
		return
	}
	n, _ := res.RowsAffected()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "cleared": n})
}

type aiQuestionData struct {
	Content     string   `json:"content"`
	Material    string   `json:"material"`
	Options     []string `json:"options"`
	Answer      string   `json:"answer"`
	AnswerIndex int64    `json:"answerIndex"`
}

func (s *Server) aiExplain(w http.ResponseWriter, r *http.Request) {
	var req struct {
		QuestionID   any             `json:"questionId"`
		Selected     json.RawMessage `json:"selected"`
		Correct      json.RawMessage `json:"correct"`
		QuestionData *aiQuestionData `json:"questionData"`
	}
	if err := decodeBody(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "请求格式不正确")
		return
	}
	id := stringifyID(req.QuestionID)
	var q *questionJSON
	if strings.HasPrefix(id, "custom-") {
		cq, err := s.findCustomQuestion(r, id)
		if err != nil {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "题目不存在")
			return
		}
		tmp := customToQuestionJSON(cq)
		q = &tmp
	} else if req.QuestionData != nil && strings.TrimSpace(req.QuestionData.Content) != "" {
		q = &questionJSON{Content: req.QuestionData.Content, Options: req.QuestionData.Options, Answer: req.QuestionData.Answer, AnswerIndex: req.QuestionData.AnswerIndex}
	} else {
		var err error
		q, err = s.findQuestion(r, id)
		if err != nil {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "题目不存在")
			return
		}
	}
	agent, err := s.ai.GetByRole("xingce-explainer")
	if err != nil || agent == nil {
		writeJSON(w, http.StatusOK, map[string]any{"notice": "行测解析 AI 未配置"})
		return
	}
	if agent.Enabled == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"notice": "行测解析 AI 未启用，请到「AI 设置」页面启用。"})
		return
	}
	if agent.APIKey == "" {
		writeJSON(w, http.StatusOK, map[string]any{"notice": "行测解析 AI 尚未配置 api_key，请到「AI 设置」页面填写后重试。"})
		return
	}
	u, _ := userFromContext(r.Context())
	selectedKey := string(req.Selected)
	if selectedKey == "" {
		selectedKey = "none"
	}
	correctKey := string(req.Correct)
	if correctKey == "" {
		correctKey = "none"
	}
	var cached string
	err = s.db.QueryRowContext(r.Context(), `
		SELECT content FROM ai_explains
		WHERE user_id = ? AND question_id = ? AND selected = ? AND correct = ?`,
		u.ID, id, selectedKey, correctKey).Scan(&cached)
	if err == nil && cached != "" {
		writeJSON(w, http.StatusOK, map[string]any{"content": cached, "cached": true})
		return
	}
	prompt := buildExplainPrompt(q, string(req.Selected), string(req.Correct))
	content, err := s.ai.Call(s.agentForUser(r, agent), prompt)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"notice": "AI 解析失败：" + err.Error()})
		return
	}
	_, _ = s.db.ExecContext(r.Context(), `
		INSERT INTO ai_explains (user_id, question_id, selected, correct, content, model)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id, question_id, selected, correct) DO UPDATE SET content = excluded.content, model = excluded.model`,
		u.ID, id, selectedKey, correctKey, content, agent.Model)
	writeJSON(w, http.StatusOK, map[string]any{"content": content, "cached": false})
}

func buildExplainPrompt(q *questionJSON, selected, correct string) string {
	var b strings.Builder
	b.WriteString("请解析下面这道考公题目。\n\n【题干】\n")
	b.WriteString(q.Content)
	if len(q.Options) > 0 {
		b.WriteString("\n\n【选项】\n")
		for i, o := range q.Options {
			b.WriteString(fmt.Sprintf("%c. %s\n", 'A'+i, o))
		}
	}
	if q.Answer != "" {
		b.WriteString("\n【正确答案】" + q.Answer + "\n")
	}
	if selected != "" && selected != "null" {
		b.WriteString("\n【考生选择】" + selected + "\n")
	}
	if correct != "" && correct != "null" {
		b.WriteString("\n【是否正确】" + correct + "\n")
	}
	b.WriteString("\n请按【考点】【正确项解析】【错误项排除】【解题技巧】输出。")
	return b.String()
}

func (s *Server) aiOCR(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Image   string `json:"image"`
		Subject string `json:"subject"`
	}
	if err := decodeBody(w, r, &req); err != nil || req.Image == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "缺少图片")
		return
	}
	agent, err := s.ai.GetByRole("image-reader")
	if err != nil || agent == nil || agent.APIKey == "" || agent.Enabled == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"notice": "识图转写员未启用或尚未配置 api_key"})
		return
	}
	if len(req.Image) > 25<<20 {
		writeError(w, http.StatusBadRequest, "TOO_LARGE", "图片过大")
		return
	}
	text, err := s.ai.CallVision(s.agentForUser(r, agent), "请逐字、完整、准确地转写这张图片中的全部文字、公式、图表和图形信息，不要解题。", []string{req.Image})
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"notice": "OCR 失败：" + err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"text": text})
}

func (s *Server) aiStructure(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Text  string `json:"text"`
		Image string `json:"image"`
	}
	if err := decodeBody(w, r, &req); err != nil || (req.Text == "" && req.Image == "") {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "缺少文本或图片")
		return
	}
	agent, err := s.ai.GetByRole("custom-question-parser")
	if err != nil || agent == nil || agent.APIKey == "" || agent.Enabled == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"notice": "题目解析员未启用或尚未配置 api_key"})
		return
	}
	var text string
	if req.Image != "" {
		text, err = s.ai.CallVision(s.agentForUser(r, agent), "把图片中的题目整理成结构化 JSON，严格遵循系统提示中的字段和规则。", []string{req.Image})
	} else {
		text, err = s.ai.Call(s.agentForUser(r, agent), req.Text)
	}
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"notice": "题目解析失败：" + err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"text": text})
}

func (s *Server) aiGrade(w http.ResponseWriter, r *http.Request) {
	var req struct {
		QuestionID int64  `json:"questionId"`
		Content    string `json:"content"`
		Material   string `json:"material"`
	}
	if err := decodeBody(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "请求格式不正确")
		return
	}
	agent, err := s.ai.GetByRole("shenlun-grader")
	if err != nil || agent == nil || agent.APIKey == "" || agent.Enabled == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"notice": "申论/综应批改 AI 未启用或尚未配置 api_key", "score": nil})
		return
	}
	var q *questionJSON
	if req.QuestionID > 0 {
		q, _ = s.findQuestion(r, strconv.FormatInt(req.QuestionID, 10))
	}
	fullScore := ""
	if q != nil {
		fullScore = extractFullScore(q.Content)
	}
	material := req.Material
	if material == "" && q != nil {
		material = s.materialTextForQuestion(r, q)
	}
	prompt := "请按系统提示中的申论/综应评分规则批改下面的作答。\n"
	if q != nil {
		prompt += "\n【题目】\n" + q.Content
	}
	if fullScore != "" {
		prompt += "\n\n【满分】" + fullScore + " 分"
	}
	if material != "" {
		prompt += "\n\n【给定材料】\n" + trimRunes(material, 8000)
	} else {
		prompt += "\n\n【注意】本题给定材料暂未关联，请基于题目要求评卷，并在结论中说明。"
	}
	prompt += "\n\n【用户作答】\n" + req.Content
	result, err := s.ai.Call(s.agentForUser(r, agent), prompt)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"notice": "批改失败：" + err.Error(), "score": nil})
		return
	}
	if q != nil {
		u, _ := userFromContext(r.Context())
		subject := subjectOfPaper(r, s, q.PaperID)
		_, _ = s.db.ExecContext(r.Context(), `
			INSERT INTO practice_records
			(user_id, question_id, paper_id, subject, chapter, question_type, selected, is_correct, cost_ms, group_key, sub_key)
			VALUES (?, ?, ?, ?, ?, ?, ?, NULL, 0, ?, '')`,
			u.ID, strconv.FormatInt(q.QuestionID, 10), q.PaperID, subject, q.Chapter, q.Type,
			mustJSON(map[string]string{"answer": req.Content}), groupForSubject(subject))
	}
	writeJSON(w, http.StatusOK, map[string]any{"notice": "批改完成", "score": nil, "result": result, "fullScore": fullScore})
}

var fullScoreRE = regexp.MustCompile(`[（(]\s*(\d{1,2})\s*分\s*[）)]`)

func extractFullScore(content string) string {
	m := fullScoreRE.FindStringSubmatch(content)
	if len(m) == 2 {
		return m[1]
	}
	return ""
}

func (s *Server) materialTextForQuestion(r *http.Request, q *questionJSON) string {
	if q.PaperID > 0 {
		rows, err := s.db.QueryContext(r.Context(), `SELECT title, text FROM materials WHERE paperId = ? ORDER BY idx`, q.PaperID)
		if err == nil {
			var parts []string
			for rows.Next() {
				var title, text string
				if rows.Scan(&title, &text) == nil {
					parts = append(parts, title+"\n"+text)
				}
			}
			rows.Close()
			if len(parts) > 0 {
				return strings.Join(parts, "\n\n")
			}
		}
	}
	var content string
	_ = s.db.QueryRowContext(r.Context(), `
		SELECT m.content FROM q_materials m
		JOIN q_material_map qm ON qm.material_id = m.material_id
		WHERE qm.question_id = ? LIMIT 1`, q.QuestionID).Scan(&content)
	return htmlTagRE.ReplaceAllString(content, " ")
}

func (s *Server) aiMaterial(w http.ResponseWriter, r *http.Request) {
	paperID := queryInt64(r, "paperId", 0)
	if paperID <= 0 {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "缺少 paperId")
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT title, idx, text FROM materials WHERE paperId = ? ORDER BY idx`, paperID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "读取材料失败")
		return
	}
	defer rows.Close()
	blocks := make([]materialBlock, 0)
	var parts []string
	for rows.Next() {
		var b materialBlock
		if rows.Scan(&b.Title, &b.Idx, &b.Text) == nil {
			blocks = append(blocks, b)
			parts = append(parts, b.Title+"\n"+b.Text)
		}
	}
	if len(blocks) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"text": nil, "notice": "该试卷暂无材料"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"text": strings.Join(parts, "\n\n"), "blocks": blocks, "cached": true, "source": "materials"})
}

func (s *Server) aiModelsProxy(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BaseURL string `json:"baseUrl"`
		APIKey  string `json:"apiKey"`
	}
	if err := decodeBody(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "请求格式不正确")
		return
	}
	base := strings.TrimRight(strings.TrimSpace(req.BaseURL), "/")
	if base == "" || !safeOutboundURL(base) {
		writeError(w, http.StatusBadRequest, "BAD_URL", "Base URL 不合法")
		return
	}
	base = strings.TrimSuffix(base, "/chat/completions")
	candidates := []string{base + "/models"}
	if !strings.HasSuffix(base, "/v1") {
		candidates = append(candidates, base+"/v1/models")
	}
	client := safeHTTPClient(30 * time.Second)
	for _, u := range candidates {
		req2, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, u, nil)
		req2.Header.Set("Accept", "application/json")
		if req.APIKey != "" {
			req2.Header.Set("Authorization", "Bearer "+req.APIKey)
		}
		res, err := client.Do(req2)
		if err != nil {
			continue
		}
		body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
		res.Body.Close()
		if res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "API Key 不正确")
			return
		}
		if res.StatusCode >= 200 && res.StatusCode < 300 {
			var data struct {
				Data []any `json:"data"`
			}
			if json.Unmarshal(body, &data) == nil && data.Data != nil {
				writeJSON(w, http.StatusOK, data)
				return
			}
		}
	}
	writeError(w, http.StatusBadGateway, "UPSTREAM", "获取模型列表失败")
}

func safeHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			if !safeOutboundURL(req.URL.String()) {
				return fmt.Errorf("redirect to private address is not allowed")
			}
			return nil
		},
	}
}

func safeOutboundURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return false
	}
	ips, err := net.LookupIP(u.Hostname())
	if err != nil {
		return false
	}
	for _, ip := range ips {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
			return false
		}
	}
	return true
}

func (s *Server) agentForUser(r *http.Request, agent *ai.Agent) *ai.Agent {
	copyAgent := *agent
	u, _ := userFromContext(r.Context())
	if u != nil && strings.TrimSpace(agent.Skill) != "" {
		var text string
		err := s.db.QueryRowContext(r.Context(),
			`SELECT text FROM user_skills WHERE user_id = ? AND name = ?`, u.ID, agent.Skill).Scan(&text)
		if err == nil && strings.TrimSpace(text) != "" {
			copyAgent.Skill = text
		} else if builtin, info := s.ai.ResolveSkill(agent.Skill); info != nil {
			copyAgent.Skill = builtin
		}
	}
	return &copyAgent
}

func customToQuestionJSON(q *customQuestionRow) questionJSON {
	var options []string
	_ = json.Unmarshal([]byte(q.Options), &options)
	return questionJSON{Content: q.Prompt, Options: options, Answer: q.Answer, AnswerIndex: q.AnswerIndex, Analysis: q.Analysis}
}

func trimRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
