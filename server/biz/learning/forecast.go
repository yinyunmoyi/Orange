package learning

import "time"

type ForecastBucket struct {
	Date  time.Time
	Count int64
}

func ForecastWindow(now time.Time, loc *time.Location, days int) (time.Time, time.Time) {
	local := now.In(loc)
	today := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
	start := today.AddDate(0, 0, 1)
	return start, start.AddDate(0, 0, days)
}

func BuildForecast(now time.Time, loc *time.Location, days int, reviewTimes []time.Time) []ForecastBucket {
	if days <= 0 {
		return []ForecastBucket{}
	}
	start, end := ForecastWindow(now, loc, days)
	counts := make(map[string]int64, days)
	for _, reviewTime := range reviewTimes {
		local := reviewTime.In(loc)
		day := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
		if day.Before(start) || !day.Before(end) {
			continue
		}
		counts[day.Format("2006-01-02")]++
	}
	result := make([]ForecastBucket, 0, days)
	for offset := 0; offset < days; offset++ {
		day := start.AddDate(0, 0, offset)
		result = append(result, ForecastBucket{
			Date:  day,
			Count: counts[day.Format("2006-01-02")],
		})
	}
	return result
}
