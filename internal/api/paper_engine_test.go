package api

import "testing"

func TestAllocateWeightsSum(t *testing.T) {
	got := allocateWeights([]float64{1, 1, 1, 1}, 130)
	sum := 0
	for _, n := range got {
		sum += n
	}
	if sum != 130 {
		t.Fatalf("sum = %d, want 130", sum)
	}
}

func TestTrimPaperPicksKeepsGroup(t *testing.T) {
	picks := []paperPick{
		{QuestionID: 1, Module: "资料分析", GroupID: "g1", GroupIndex: 0, GroupTotal: 5},
		{QuestionID: 2, Module: "资料分析", GroupID: "g1", GroupIndex: 1, GroupTotal: 5},
		{QuestionID: 3, Module: "资料分析", GroupID: "g1", GroupIndex: 2, GroupTotal: 5},
		{QuestionID: 4, Module: "资料分析", GroupID: "g1", GroupIndex: 3, GroupTotal: 5},
		{QuestionID: 5, Module: "资料分析", GroupID: "g1", GroupIndex: 4, GroupTotal: 5},
		{QuestionID: 6, Module: "常识判断", SubIdx: 0},
		{QuestionID: 7, Module: "常识判断", SubIdx: 0},
		{QuestionID: 8, Module: "常识判断", SubIdx: 0},
		{QuestionID: 9, Module: "常识判断", SubIdx: 0},
		{QuestionID: 10, Module: "常识判断", SubIdx: 0},
	}
	out := trimPaperPicks(picks, 8)
	if len(out) != 8 {
		t.Fatalf("len = %d, want 8", len(out))
	}
	groupCount := 0
	for _, p := range out {
		if p.GroupID == "g1" {
			groupCount++
		}
	}
	if groupCount != 5 {
		t.Fatalf("group members = %d, want 5", groupCount)
	}
}
