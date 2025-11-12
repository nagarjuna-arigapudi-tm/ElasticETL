package pipeline

import (
	"context"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	"elasticetl/pkg/config"
)

// Scheduler handles pipeline scheduling with support for startTime/interval and cron schedules
type Scheduler struct {
	config        config.ScheduleConfig
	cronScheduler *cron.Cron
	ticker        *time.Ticker
	stopChan      chan struct{}
	executeChan   chan struct{}
	running       bool
	// Add sync primitives for safe cleanup
	stopOnce      sync.Once
	goroutineDone chan struct{}
}

// NewScheduler creates a new scheduler based on the schedule configuration
func NewScheduler(scheduleConfig config.ScheduleConfig, legacyInterval time.Duration) *Scheduler {
	scheduler := &Scheduler{
		config:        scheduleConfig,
		stopChan:      make(chan struct{}),
		executeChan:   make(chan struct{}, 1), // Buffered to prevent blocking
		goroutineDone: make(chan struct{}),
	}

	// Determine the effective schedule configuration
	effectiveConfig := scheduler.getEffectiveScheduleConfig(legacyInterval)
	scheduler.config = effectiveConfig

	return scheduler
}

// getEffectiveScheduleConfig determines the effective schedule configuration
func (s *Scheduler) getEffectiveScheduleConfig(legacyInterval time.Duration) config.ScheduleConfig {
	// Priority 1: CronSchedule takes precedence
	if s.config.CronSchedule != "" {
		return s.config
	}

	// Priority 2: New schedule configuration (startTime + interval)
	if s.config.Interval > 0 {
		return s.config
	}

	// Priority 3: Legacy interval configuration
	if legacyInterval > 0 {
		return config.ScheduleConfig{
			Interval: legacyInterval,
		}
	}

	// Default: 10 minutes interval, start immediately
	return config.ScheduleConfig{
		Interval: 10 * time.Minute,
	}
}

// Start starts the scheduler
func (s *Scheduler) Start(ctx context.Context, executeFunc func()) error {
	if s.running {
		return fmt.Errorf("scheduler is already running")
	}

	s.running = true

	// Handle cron schedule
	if s.config.CronSchedule != "" {
		return s.startCronSchedule(ctx, executeFunc)
	}

	// Handle startTime + interval schedule
	return s.startIntervalSchedule(ctx, executeFunc)
}

// startCronSchedule starts cron-based scheduling
func (s *Scheduler) startCronSchedule(ctx context.Context, executeFunc func()) error {
	s.cronScheduler = cron.New(cron.WithSeconds()) // Support seconds in cron expressions

	_, err := s.cronScheduler.AddFunc(s.config.CronSchedule, func() {
		// Execute directly in the cron callback to avoid channel issues
		// Use a separate goroutine to prevent blocking the cron scheduler
		go func() {
			// Check if context is still valid before executing
			select {
			case <-ctx.Done():
				return
			case <-s.stopChan:
				return
			default:
				executeFunc()
			}
		}()
	})

	if err != nil {
		s.running = false
		return fmt.Errorf("invalid cron schedule '%s': %w", s.config.CronSchedule, err)
	}

	s.cronScheduler.Start()

	// Start a monitoring goroutine to handle context cancellation and cleanup
	go func() {
		defer func() {
			// Signal that this goroutine is done
			select {
			case s.goroutineDone <- struct{}{}:
			default:
			}
		}()

		// Wait for context cancellation or stop signal
		select {
		case <-ctx.Done():
			s.cronScheduler.Stop()
		case <-s.stopChan:
			s.cronScheduler.Stop()
		}
	}()

	return nil
}

// startIntervalSchedule starts interval-based scheduling with optional start time
func (s *Scheduler) startIntervalSchedule(ctx context.Context, executeFunc func()) error {
	interval := s.config.Interval
	if interval <= 0 {
		interval = 10 * time.Minute // Default interval
	}

	// Calculate initial delay based on startTime
	initialDelay := s.calculateInitialDelay()

	// Start ticker goroutine with proper cleanup
	go func() {
		defer func() {
			// Signal that this goroutine is done
			select {
			case s.goroutineDone <- struct{}{}:
			default:
			}
		}()

		// Wait for initial delay if startTime is specified
		if initialDelay > 0 {
			timer := time.NewTimer(initialDelay)
			defer timer.Stop()

			select {
			case <-timer.C:
				// Continue to start execution
			case <-ctx.Done():
				return
			case <-s.stopChan:
				return
			}
		}

		// Execute immediately after initial delay (or immediately if no startTime)
		select {
		case s.executeChan <- struct{}{}:
		default:
		}

		// Start ticker for periodic execution
		s.ticker = time.NewTicker(interval)
		defer s.ticker.Stop()

		for {
			select {
			case <-s.ticker.C:
				select {
				case s.executeChan <- struct{}{}:
				default:
					// Channel is full, skip this execution
				}
			case <-ctx.Done():
				return
			case <-s.stopChan:
				return
			}
		}
	}()

	// Start execution handler with proper cleanup
	go func() {
		defer func() {
			// Signal that this goroutine is done
			select {
			case s.goroutineDone <- struct{}{}:
			default:
			}
		}()
		s.handleExecution(ctx, executeFunc)
	}()

	return nil
}

// calculateInitialDelay calculates the delay based on startTime or initialWait
func (s *Scheduler) calculateInitialDelay() time.Duration {
	// Priority 1: StartTime takes precedence over initialWait
	if s.config.StartTime != "" {
		startTime, err := s.parseStartTime(s.config.StartTime)
		if err != nil {
			// Invalid start time, fall back to initialWait logic
			return s.calculateInitialWaitDelay()
		}

		now := time.Now()
		today := time.Date(now.Year(), now.Month(), now.Day(), startTime.Hour(), startTime.Minute(), startTime.Second(), 0, now.Location())

		if today.Before(now) {
			// Start time has passed today, schedule for tomorrow
			today = today.Add(24 * time.Hour)
		}

		return today.Sub(now)
	}

	// Priority 2: Use initialWait when no startTime is specified
	return s.calculateInitialWaitDelay()
}

// calculateInitialWaitDelay calculates delay based on initialWait parameter
func (s *Scheduler) calculateInitialWaitDelay() time.Duration {
	// If initialWait is not specified (nil), use random wait between 0-3 minutes
	if s.config.InitialWait == nil {
		randomSeconds := rand.Intn(180) // 0 to 179 seconds (0 to 2:59 minutes)
		return time.Duration(randomSeconds) * time.Second
	}

	// If initialWait is explicitly set to 0, start immediately
	if *s.config.InitialWait == 0 {
		return 0
	}

	// If initialWait is specified and > 0, use it
	return *s.config.InitialWait
}

// parseStartTime parses start time in HH:MM:SS format
func (s *Scheduler) parseStartTime(startTimeStr string) (time.Time, error) {
	parts := strings.Split(startTimeStr, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return time.Time{}, fmt.Errorf("invalid start time format, expected HH:MM or HH:MM:SS")
	}

	hour, err := strconv.Atoi(parts[0])
	if err != nil || hour < 0 || hour > 23 {
		return time.Time{}, fmt.Errorf("invalid hour: %s", parts[0])
	}

	minute, err := strconv.Atoi(parts[1])
	if err != nil || minute < 0 || minute > 59 {
		return time.Time{}, fmt.Errorf("invalid minute: %s", parts[1])
	}

	second := 0
	if len(parts) == 3 {
		second, err = strconv.Atoi(parts[2])
		if err != nil || second < 0 || second > 59 {
			return time.Time{}, fmt.Errorf("invalid second: %s", parts[2])
		}
	}

	// Return a time with the parsed components (date doesn't matter for calculation)
	return time.Date(2000, 1, 1, hour, minute, second, 0, time.UTC), nil
}

// handleExecution handles execution requests with retry logic
func (s *Scheduler) handleExecution(ctx context.Context, executeFunc func()) {
	for {
		select {
		case <-s.executeChan:
			executeFunc()
		case <-ctx.Done():
			return
		case <-s.stopChan:
			return
		}
	}
}

// Stop stops the scheduler and waits for all goroutines to finish
func (s *Scheduler) Stop() {
	s.stopOnce.Do(func() {
		if !s.running {
			return
		}

		s.running = false

		// Stop cron scheduler first
		if s.cronScheduler != nil {
			s.cronScheduler.Stop()
		}

		// Stop ticker
		if s.ticker != nil {
			s.ticker.Stop()
		}

		// Signal all goroutines to stop
		close(s.stopChan)

		// Wait for goroutines to finish with timeout to prevent hanging
		timeout := time.NewTimer(5 * time.Second)
		defer timeout.Stop()

		goroutinesFinished := 0
		expectedGoroutines := 1 // monitoring goroutine for cron or handleExecution for interval
		if s.config.CronSchedule == "" {
			expectedGoroutines = 2 // handleExecution + ticker goroutine for interval scheduling
		}

		for goroutinesFinished < expectedGoroutines {
			select {
			case <-s.goroutineDone:
				goroutinesFinished++
			case <-timeout.C:
				// Timeout reached, force exit to prevent hanging
				return
			}
		}

		// Close channels to prevent memory leaks
		close(s.executeChan)
		close(s.goroutineDone)
	})
}

// IsRunning returns whether the scheduler is running
func (s *Scheduler) IsRunning() bool {
	return s.running
}

// GetScheduleInfo returns information about the current schedule
func (s *Scheduler) GetScheduleInfo() string {
	if s.config.CronSchedule != "" {
		return fmt.Sprintf("cron: %s", s.config.CronSchedule)
	}

	if s.config.StartTime != "" {
		return fmt.Sprintf("startTime: %s, interval: %s", s.config.StartTime, s.config.Interval)
	}

	info := fmt.Sprintf("interval: %s", s.config.Interval)
	if s.config.InitialWait != nil && *s.config.InitialWait > 0 {
		info += fmt.Sprintf(", initialWait: %s", *s.config.InitialWait)
	} else if s.config.InitialWait != nil && *s.config.InitialWait == 0 {
		info += ", initialWait: 0s (immediate)"
	} else if s.config.InitialWait == nil && s.config.StartTime == "" {
		info += ", initialWait: random(0-3min)"
	}

	return info
}
