package learning

import "testing"

func intPtr(v int) *int { return &v }

func TestPickNextPriority(t *testing.T) {
	items := []Candidate{
		{ID: 1, QueueType: QueueReview, QueueSeq: 1, Status: QueuePending},
		{ID: 2, QueueType: QueueNew, QueueSeq: 2, Status: QueuePending},
		{ID: 3, QueueType: QueueRetry, QueueSeq: 3, Status: QueueRetryWaiting, RetryAfterTurn: intPtr(9)},
	}
	if got := PickNext(items, 8); got == nil || got.ID != 2 {
		t.Fatalf("before retry due got=%+v", got)
	}
	if got := PickNext(items, 9); got == nil || got.ID != 3 {
		t.Fatalf("retry due got=%+v", got)
	}
}

func TestPickEarlyRetryWhenNothingElse(t *testing.T) {
	items := []Candidate{
		{ID: 1, QueueType: QueueRetry, QueueSeq: 1, Status: QueueRetryWaiting, RetryAfterTurn: intPtr(20)},
	}
	if got := PickNext(items, 2); got == nil || got.ID != 1 {
		t.Fatalf("got=%+v", got)
	}
}

func TestRetryTurnLeavesSevenCards(t *testing.T) {
	if got := RetryTurn(1); got != 9 {
		t.Fatalf("got=%d want=9", got)
	}
}

func TestApplySessionAnswer(t *testing.T) {
	tests := []struct {
		name       string
		sourceType string
		knownCount int
		known      bool
		want       AnswerOutcome
	}{
		{
			name: "new first known", sourceType: QueueNew, known: true,
			want: AnswerOutcome{KnownCount: 1, Requeue: true},
		},
		{
			name: "new second known", sourceType: QueueNew, knownCount: 1, known: true,
			want: AnswerOutcome{KnownCount: 2, Completed: true},
		},
		{
			name: "unknown resets new", sourceType: QueueNew, knownCount: 1, known: false,
			want: AnswerOutcome{KnownCount: 0, Requeue: true},
		},
		{
			name: "review first known", sourceType: QueueReview, known: true,
			want: AnswerOutcome{KnownCount: 1, Completed: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ApplySessionAnswer(tt.sourceType, tt.knownCount, tt.known); got != tt.want {
				t.Fatalf("ApplySessionAnswer() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestNewItemKnownCountResetsAfterUnknown(t *testing.T) {
	knownCount := 0
	steps := []struct {
		known         bool
		wantCount     int
		wantCompleted bool
	}{
		{known: true, wantCount: 1},
		{known: false, wantCount: 0},
		{known: true, wantCount: 1},
		{known: true, wantCount: 2, wantCompleted: true},
	}
	for i, step := range steps {
		outcome := ApplySessionAnswer(QueueNew, knownCount, step.known)
		if outcome.KnownCount != step.wantCount || outcome.Completed != step.wantCompleted {
			t.Fatalf("step %d outcome = %+v", i, outcome)
		}
		knownCount = outcome.KnownCount
	}
}
