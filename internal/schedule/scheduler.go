package schedule

import (
	"fmt"
	"time"

	cron "github.com/robfig/cron/v3"
)

type ScheduleStrategy interface {
	IsCron() bool
	Next(time time.Time) time.Time
}

type CronScheduleStrategy struct {
	scheduler cron.Schedule
}

func NewCronScheduleStrategy(pattern string) (*CronScheduleStrategy, error) {
	cronParser := cron.NewParser(
		cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow,
	)

	scheduler, err := cronParser.Parse(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid cron pattern '%s': %w", pattern, err)
	}

	return &CronScheduleStrategy{
		scheduler: scheduler,
	}, nil
}

func (f *CronScheduleStrategy) Next(time time.Time) time.Time {
	return f.scheduler.Next(time)
}

func (f *CronScheduleStrategy) IsCron() bool {
	return true
}

type IntervalScheduleStrategy struct {
	duration time.Duration
}

func NewIntervalScheduleStrategy(pattern string) (*IntervalScheduleStrategy, error) {
	duration, err := ParseIntervalPattern(pattern)
	if err != nil {
		return nil, err
	}
	return &IntervalScheduleStrategy{
		duration: duration,
	}, nil
}

func (f *IntervalScheduleStrategy) Next(time time.Time) time.Time {
	return time.Add(f.duration)
}

func (f *IntervalScheduleStrategy) IsCron() bool {
	return false
}

func CreateScheduler(pattern string) (ScheduleStrategy, error) {
	isCronPattern := IsCronPattern(pattern)
	if isCronPattern {
		return NewCronScheduleStrategy(pattern)
	}
	return NewIntervalScheduleStrategy(pattern)
}
