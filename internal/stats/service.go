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

func (service *Service) Summary(ctx context.Context, userID, timezone string) (Summary, error) {
	timezone = strings.TrimSpace(timezone)
	if userID == "" || service.repository == nil {
		return Summary{}, ErrInvalidStatsQuery
	}
	location, err := time.LoadLocation(timezone)
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
	summary := Summary{TotalWatches: len(events), Genres: genres, Tags: tags}
	for _, event := range events {
		media[event.MediaID] = struct{}{}
		summary.TotalMinutes += event.RuntimeMinutes
		local := event.WatchedAt.In(location)
		monthly[local.Format("2006-01")]++
		yearly[local.Format("2006")]++
		method := strings.TrimSpace(event.ViewingMethod)
		if method != "" {
			viewingMethods[method]++
		}
	}
	summary.UniqueMedia = len(media)
	summary.RepeatWatches = summary.TotalWatches - summary.UniqueMedia
	summary.Monthly = pointsFromMap(monthly, false)
	summary.Yearly = pointsFromMap(yearly, false)
	summary.Ratings = ratingPoints(ratings)
	summary.ViewingMethods = pointsFromMap(viewingMethods, true)
	return summary, nil
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
