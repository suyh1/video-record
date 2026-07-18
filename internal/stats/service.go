package stats

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

var ErrInvalidStatsQuery = errors.New("invalid stats query")

type Point struct {
	Label string
	Value int
}

type Summary struct {
	TotalWatches   int
	UniqueMedia    int
	TotalMinutes   int
	RepeatWatches  int
	Monthly        []Point
	Yearly         []Point
	Genres         []Point
	Ratings        []Point
	Tags           []Point
	ViewingMethods []Point
}

type EventFact struct {
	MediaID        string
	WatchedAt      time.Time
	RuntimeMinutes int
	ViewingMethod  string
}

type Repository interface {
	EventFacts(context.Context, string) ([]EventFact, error)
	GenreCounts(context.Context, string) ([]Point, error)
	Ratings(context.Context, string) ([]int, error)
	TagCounts(context.Context, string) ([]Point, error)
}

type Service struct {
	repository Repository
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository}
}

func (service *Service) Summary(ctx context.Context, userID, timezone, rangeKey string) (Summary, error) {
	timezone = strings.TrimSpace(timezone)
	rangeKey = strings.TrimSpace(rangeKey)
	if rangeKey == "" {
		rangeKey = "all"
	}
	if userID == "" || service.repository == nil {
		return Summary{}, ErrInvalidStatsQuery
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return Summary{}, ErrInvalidStatsQuery
	}
	now := time.Now().In(location)
	start, end, err := statsRangeBounds(rangeKey, now)
	if err != nil {
		return Summary{}, ErrInvalidStatsQuery
	}
	events, err := service.repository.EventFacts(ctx, userID)
	if err != nil {
		return Summary{}, err
	}
	genres, err := service.repository.GenreCounts(ctx, userID)
	if err != nil {
		return Summary{}, err
	}
	ratings, err := service.repository.Ratings(ctx, userID)
	if err != nil {
		return Summary{}, err
	}
	tags, err := service.repository.TagCounts(ctx, userID)
	if err != nil {
		return Summary{}, err
	}

	media := make(map[string]struct{})
	monthly := make(map[string]int)
	yearly := make(map[string]int)
	viewingMethods := make(map[string]int)
	var filtered int
	var totalMinutes int
	for _, event := range events {
		local := event.WatchedAt.In(location)
		if !start.IsZero() && local.Before(start) {
			continue
		}
		if !end.IsZero() && !local.Before(end) {
			continue
		}
		filtered++
		media[event.MediaID] = struct{}{}
		totalMinutes += event.RuntimeMinutes
		monthly[local.Format("2006-01")]++
		yearly[local.Format("2006")]++
		method := strings.TrimSpace(event.ViewingMethod)
		if method != "" {
			viewingMethods[method]++
		}
	}
	summary := Summary{
		TotalWatches: filtered,
		Genres:       genres,
		Tags:         tags,
		TotalMinutes: totalMinutes,
	}
	summary.UniqueMedia = len(media)
	summary.RepeatWatches = summary.TotalWatches - summary.UniqueMedia
	if summary.RepeatWatches < 0 {
		summary.RepeatWatches = 0
	}
	summary.Monthly = limitRecentMonths(pointsFromMap(monthly, false), 12)
	summary.Yearly = pointsFromMap(yearly, false)
	summary.Ratings = ratingPoints(ratings)
	summary.ViewingMethods = pointsFromMap(viewingMethods, true)
	return summary, nil
}

func statsRangeBounds(rangeKey string, now time.Time) (time.Time, time.Time, error) {
	switch rangeKey {
	case "all":
		return time.Time{}, time.Time{}, nil
	case "month":
		start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		return start, start.AddDate(0, 1, 0), nil
	case "year":
		start := time.Date(now.Year(), 1, 1, 0, 0, 0, 0, now.Location())
		return start, start.AddDate(1, 0, 0), nil
	default:
		if len(rangeKey) == 4 {
			year, err := time.ParseInLocation("2006", rangeKey, now.Location())
			if err != nil {
				return time.Time{}, time.Time{}, err
			}
			return year, year.AddDate(1, 0, 0), nil
		}
		return time.Time{}, time.Time{}, ErrInvalidStatsQuery
	}
}

func limitRecentMonths(points []Point, max int) []Point {
	if len(points) <= max {
		return points
	}
	return points[len(points)-max:]
}

func ratingPoints(ratings []int) []Point {
	buckets := make(map[string]int)
	for _, rating := range ratings {
		lower := rating / 10
		if rating == 100 {
			lower = 9
		}
		upper := lower
		if lower == 9 {
			buckets["9.0-10.0"]++
		} else {
			buckets[fmt.Sprintf("%d.0-%d.9", lower, upper)]++
		}
	}
	return pointsFromMap(buckets, false)
}

func pointsFromMap(values map[string]int, byValue bool) []Point {
	points := make([]Point, 0, len(values))
	for label, value := range values {
		points = append(points, Point{Label: label, Value: value})
	}
	sort.Slice(points, func(left, right int) bool {
		if byValue && points[left].Value != points[right].Value {
			return points[left].Value > points[right].Value
		}
		return points[left].Label < points[right].Label
	})
	return points
}
