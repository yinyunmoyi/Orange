package learning

const (
	QueueNew    = "new"
	QueueReview = "review"
	QueueRetry  = "retry"

	QueuePending      = "pending"
	QueueRetryWaiting = "retry_waiting"
	QueueCompleted    = "completed"
)

type AnswerOutcome struct {
	KnownCount int
	Completed  bool
	Requeue    bool
}

func ApplySessionAnswer(sourceQueueType string, knownCount int, known bool) AnswerOutcome {
	if !known {
		return AnswerOutcome{KnownCount: 0, Requeue: true}
	}
	knownCount++
	required := 1
	if sourceQueueType == QueueNew {
		required = 2
	}
	if knownCount >= required {
		return AnswerOutcome{KnownCount: knownCount, Completed: true}
	}
	return AnswerOutcome{KnownCount: knownCount, Requeue: true}
}

type Candidate struct {
	ID             int64
	QueueType      string
	QueueSeq       float64
	Status         string
	RetryAfterTurn *int
}

func RetryTurn(answerTurn int) int {
	return answerTurn + 8
}

func PickNext(items []Candidate, nextTurn int) *Candidate {
	var dueRetry, newItem, reviewItem, earlyRetry *Candidate
	for i := range items {
		item := &items[i]
		switch item.Status {
		case QueuePending:
			switch item.QueueType {
			case QueueNew:
				newItem = lowerSeq(newItem, item)
			case QueueReview:
				reviewItem = lowerSeq(reviewItem, item)
			}
		case QueueRetryWaiting:
			if item.RetryAfterTurn == nil {
				continue
			}
			if *item.RetryAfterTurn <= nextTurn {
				dueRetry = lowerRetry(dueRetry, item)
			} else {
				earlyRetry = lowerRetry(earlyRetry, item)
			}
		}
	}
	if dueRetry != nil {
		return dueRetry
	}
	if newItem != nil {
		return newItem
	}
	if reviewItem != nil {
		return reviewItem
	}
	return earlyRetry
}

func lowerSeq(current, candidate *Candidate) *Candidate {
	if current == nil || candidate.QueueSeq < current.QueueSeq {
		return candidate
	}
	return current
}

func lowerRetry(current, candidate *Candidate) *Candidate {
	if current == nil ||
		*candidate.RetryAfterTurn < *current.RetryAfterTurn ||
		(*candidate.RetryAfterTurn == *current.RetryAfterTurn && candidate.QueueSeq < current.QueueSeq) {
		return candidate
	}
	return current
}
