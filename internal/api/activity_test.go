package api

import (
	"fmt"
	"net/http"
	"testing"
	"time"
)

type activityResp struct {
	From          string `json:"from"`
	To            string `json:"to"`
	StartedOn     string `json:"startedOn"`
	ActiveDays    int64  `json:"activeDays"`
	CurrentStreak int64  `json:"currentStreak"`
	LongestStreak int64  `json:"longestStreak"`
	Total         int64  `json:"total"`
	Correct       int64  `json:"correct"`
	Graded        int64  `json:"graded"`
	CostMs        int64  `json:"ms"`
	Days          []struct {
		Day      string `json:"d"`
		Count    int64  `json:"c"`
		OK       int64  `json:"ok"`
		Graded   int64  `json:"graded"`
		CostMs   int64  `json:"ms"`
		Subjects []struct {
			Name   string `json:"name"`
			Count  int64  `json:"c"`
			OK     int64  `json:"ok"`
			Graded int64  `json:"graded"`
		} `json:"subjects"`
	} `json:"days"`
	Subjects []struct {
		Name   string `json:"name"`
		Count  int64  `json:"c"`
		OK     int64  `json:"ok"`
		Graded int64  `json:"graded"`
	} `json:"subjects"`
}

func insertRecord(t *testing.T, srv *Server, userID int64, subject string, correct int, costMs int, offset string) {
	t.Helper()
	_, err := srv.db.Exec(`
		INSERT INTO practice_records
			(user_id, question_id, subject, chapter, question_type, selected, is_correct, cost_ms, created_at)
		VALUES (?, ?, ?, '测试', 1, '[]', ?, ?,
		        datetime('now', 'localtime', ?))`,
		userID, fmt.Sprintf("q-%s-%d", offset, time.Now().UnixNano()),
		subject, correct, costMs, offset)
	if err != nil {
		t.Fatal(err)
	}
}

func TestActivityEmptyAndIsolated(t *testing.T) {
	ts, srv := newTestServer(t)
	client := testClient(t, ts)

	res, err := client.Get(ts.URL + "/api/records/activity")
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d, want 401", res.StatusCode)
	}
	res.Body.Close()

	loginTestUser(t, ts, client, "admin", "admin123456")

	// A second account's records must not leak into admin's wall.
	if _, err := srv.db.Exec(`INSERT INTO users (username, password_hash, role) VALUES ('other', 'x', 'user')`); err != nil {
		t.Fatal(err)
	}
	var otherID int64
	if err := srv.db.QueryRow(`SELECT id FROM users WHERE username = 'other'`).Scan(&otherID); err != nil {
		t.Fatal(err)
	}
	insertRecord(t, srv, otherID, "公务员·行测", 1, 9000, "-0 day")

	res, err = client.Get(ts.URL + "/api/records/activity")
	if err != nil {
		t.Fatal(err)
	}
	var empty activityResp
	decodeJSON(t, res, &empty)
	if empty.Total != 0 || empty.ActiveDays != 0 || empty.CurrentStreak != 0 || len(empty.Days) != 0 {
		t.Fatalf("empty wall = %+v, want zeroed", empty)
	}
	if empty.StartedOn != "" {
		t.Fatalf("startedOn = %q, want empty", empty.StartedOn)
	}
}

func TestActivityAggregatesByDayAndStreak(t *testing.T) {
	ts, srv := newTestServer(t)
	client := testClient(t, ts)
	loginTestUser(t, ts, client, "admin", "admin123456")
	var adminID int64
	if err := srv.db.QueryRow(`SELECT id FROM users WHERE username = 'admin'`).Scan(&adminID); err != nil {
		t.Fatal(err)
	}

	insertRecord(t, srv, adminID, "公务员·行测", 1, 60000, "-0 day")
	insertRecord(t, srv, adminID, "公务员·行测", 0, 0, "-0 day")
	insertRecord(t, srv, adminID, "公务员·申论", 1, 30000, "-0 day")
	insertRecord(t, srv, adminID, "公务员·行测", 1, 0, "-1 day")
	insertRecord(t, srv, adminID, "公务员·行测", 0, 0, "-2 day")
	insertRecord(t, srv, adminID, "事业编·职测", 1, 0, "-10 day")
	insertRecord(t, srv, adminID, "事业编·职测", 1, 0, "-9 day")

	res, err := client.Get(ts.URL + "/api/records/activity?days=30")
	if err != nil {
		t.Fatal(err)
	}
	var got activityResp
	decodeJSON(t, res, &got)

	if got.Total != 7 || got.Correct != 5 || got.Graded != 7 {
		t.Fatalf("totals = %d/%d/%d, want 7/5/7", got.Total, got.Correct, got.Graded)
	}
	if got.CostMs != 90000 {
		t.Fatalf("cost = %d, want 90000", got.CostMs)
	}
	if got.ActiveDays != 5 {
		t.Fatalf("activeDays = %d, want 5", got.ActiveDays)
	}
	if got.CurrentStreak != 3 {
		t.Fatalf("currentStreak = %d, want 3", got.CurrentStreak)
	}
	if got.LongestStreak != 3 {
		t.Fatalf("longestStreak = %d, want 3", got.LongestStreak)
	}
	wantStart := time.Now().AddDate(0, 0, -10).Format("2006-01-02")
	if got.StartedOn != wantStart {
		t.Fatalf("startedOn = %q, want %q", got.StartedOn, wantStart)
	}
	if len(got.Days) != 5 {
		t.Fatalf("days = %d, want 5", len(got.Days))
	}

	today := got.Days[len(got.Days)-1]
	wantToday := time.Now().Format("2006-01-02")
	if today.Day != wantToday {
		t.Fatalf("last day = %q, want %q", today.Day, wantToday)
	}
	if today.Count != 3 || today.OK != 2 || today.CostMs != 90000 {
		t.Fatalf("today = %+v, want 3/2/90000", today)
	}
	if len(today.Subjects) != 2 || today.Subjects[0].Name != "公务员·行测" || today.Subjects[0].Count != 2 {
		t.Fatalf("today subjects = %+v, want 行测 first with 2", today.Subjects)
	}

	if len(got.Subjects) != 3 {
		t.Fatalf("subject totals = %+v, want 3 rows", got.Subjects)
	}
	if got.Subjects[0].Name != "公务员·行测" || got.Subjects[0].Count != 4 || got.Subjects[0].OK != 2 {
		t.Fatalf("top subject = %+v, want 行测 4/2", got.Subjects[0])
	}

	// A narrower window keeps today's streak but drops the older run.
	res, err = client.Get(ts.URL + "/api/records/activity?days=3")
	if err != nil {
		t.Fatal(err)
	}
	var narrow activityResp
	decodeJSON(t, res, &narrow)
	if narrow.Total != 5 || narrow.ActiveDays != 3 || len(narrow.Days) != 3 {
		t.Fatalf("3-day window = total %d active %d days %d, want 5/3/3", narrow.Total, narrow.ActiveDays, len(narrow.Days))
	}
}
