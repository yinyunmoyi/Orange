package learning

import (
	"fmt"
	"time"
)

const (
	StatusNotStarted = "not_started"
	StatusLearning   = "learning"
	StatusLearned    = "learned"
	StatusMastered   = "mastered"
)

type Progress struct {
	Status            string
	Stage             int
	ReviewSuccessDays int
	NextReviewAt      *time.Time
	LastReviewedAt    *time.Time
	LearnedAt         *time.Time
	MasteredAt        *time.Time
}

var stageIntervals = map[int]int{
	0: 1,
	1: 3,
	2: 3,
	3: 7,
	4: 16,
	5: 30,
}

func NextStudyDay(now time.Time, days int, loc *time.Location) time.Time {
	local := now.In(loc).AddDate(0, 0, days)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
}

// ExpandReviewDates 从给定的 stage 和"下一次复习日期"起，按 stageIntervals 依次展开
// 该词的剩余复习日期序列（每一项对应一次将要发生的复习）。
// stage 6 的入口也会有一次复习，通过后才置为 Learned；因此本函数会一直展开到 stage=6 那次为止。
func ExpandReviewDates(stage int, nextReviewAt time.Time, loc *time.Location) []time.Time {
	if stage < 0 || stage > 6 {
		return nil
	}
	dates := make([]time.Time, 0, 7-stage)
	current := nextReviewAt
	for s := stage; s <= 6; s++ {
		dates = append(dates, current)
		if s == 6 {
			break
		}
		current = NextStudyDay(current, stageIntervals[s], loc)
	}
	return dates
}

func ApplyUnknown(p Progress, now time.Time, loc *time.Location) Progress {
	next := NextStudyDay(now, 1, loc)
	p.Status = StatusLearning
	p.Stage = 0
	p.ReviewSuccessDays = 0
	p.NextReviewAt = &next
	p.LastReviewedAt = &now
	p.LearnedAt = nil
	return p
}

func ApplyMastered(p Progress, now time.Time) Progress {
	p.Status = StatusMastered
	p.NextReviewAt = nil
	p.LastReviewedAt = &now
	p.MasteredAt = &now
	return p
}

func ApplyKnown(p Progress, now time.Time, loc *time.Location) (Progress, error) {
	if p.Stage < 0 || p.Stage > 6 {
		return p, fmt.Errorf("invalid memory stage: %d", p.Stage)
	}
	p.LastReviewedAt = &now
	if p.Stage == 6 {
		p.Status = StatusLearned
		p.ReviewSuccessDays++
		p.NextReviewAt = nil
		p.LearnedAt = &now
		return p, nil
	}
	days := stageIntervals[p.Stage]
	next := NextStudyDay(now, days, loc)
	p.Status = StatusLearning
	p.Stage++
	p.ReviewSuccessDays++
	p.NextReviewAt = &next
	p.LearnedAt = nil
	return p, nil
}
