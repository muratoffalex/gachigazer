package queue

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/muratoffalex/gachigazer/internal/commands"
	"github.com/muratoffalex/gachigazer/internal/database"
	"github.com/muratoffalex/gachigazer/internal/logger"
	"github.com/muratoffalex/gachigazer/internal/telegram"
	"golang.org/x/time/rate"
)

type TaskStatus string

const (
	TaskStatusPending  TaskStatus = "pending"
	TaskStatusRunning  TaskStatus = "running"
	TaskStatusComplete TaskStatus = "complete"
	TaskStatusFailed   TaskStatus = "failed"
)

type Task struct {
	ID          int64
	Command     string
	UpdateData  []byte
	RetryCount  int
	MaxRetries  int
	RetryDelay  time.Duration
	LastAttempt time.Time
	NextAttempt time.Time
	Status      TaskStatus
	Update      *telegram.Update
}

func (t *Task) GetUpdate() (*telegram.Update, error) {
	if t.Update != nil {
		return t.Update, nil
	}

	var update telegram.Update
	if err := json.Unmarshal(t.UpdateData, &update); err != nil {
		return nil, fmt.Errorf("failed to unmarshal update data: %w", err)
	}
	t.Update = &update
	return t.Update, nil
}

type Queue struct {
	db                database.Database
	commandLimiters   map[string]*rate.Limiter
	commandSemaphores map[string]chan struct{}
	logger            logger.Logger
	handlers          map[string]commands.Command
}

func NewQueue(db database.Database, logger logger.Logger) *Queue {
	return &Queue{
		db:                db,
		commandLimiters:   make(map[string]*rate.Limiter),
		commandSemaphores: make(map[string]chan struct{}),
		logger:            logger,
		handlers:          make(map[string]commands.Command),
	}
}

func (q *Queue) RegisterHandlers(handlers map[string]commands.Command) {
	q.handlers = handlers
}

func (q *Queue) Add(cmd commands.Command, update telegram.Update, maxRetries int, retryDelay int64) error {
	cmdName := cmd.Name()
	if cmdName == "" {
		return fmt.Errorf("command name cannot be empty")
	}

	q.logger.WithFields(logger.Fields{
		"command":   cmdName,
		"update_id": update.UpdateID,
	}).Debug("Adding task to queue")

	updateData, err := json.Marshal(update)
	if err != nil {
		return err
	}

	_, err = q.db.ExecWithRetry(context.Background(), `
        INSERT INTO tasks (command, update_data, max_retries, retry_delay, next_attempt)
        VALUES (?, ?, ?, ?, ?)
    `, cmdName, updateData, maxRetries, retryDelay, time.Now())
	if err != nil {
		q.logger.WithError(err).
			WithField("command", cmdName).
			Error("Failed to add task")
		return err
	}

	q.logger.WithField("command", cmdName).Debug("Task added successfully")
	return nil
}

func (q *Queue) Start(ctx context.Context, handlers map[string]commands.Command) {
	q.logger.WithFields(logger.Fields{
		"handlers": handlers,
	}).Info("Starting queue with handlers")

	q.handlers = handlers
	for cmd, handler := range handlers {
		cfg := handler.GetQueueConfig()

		interval := cfg.Throttle.Period / time.Duration(cfg.Throttle.Requests)
		q.logger.WithFields(logger.Fields{
			"command":     cmd,
			"period":      cfg.Throttle.Period,
			"requests":    cfg.Throttle.Requests,
			"interval":    interval,
			"concurrency": cfg.Throttle.Concurrency,
		}).Info("Configured rate limiter")

		q.commandLimiters[cmd] = rate.NewLimiter(
			rate.Every(interval),
			cfg.Throttle.Requests,
		)
		q.commandSemaphores[cmd] = make(chan struct{}, cfg.Throttle.Concurrency)
		go q.processCommandQueue(ctx, cmd, handler)
	}
}

func (q *Queue) handleTaskError(ctx context.Context, task Task) error {
	log := q.logger.WithFields(logger.Fields{
		"command":     task.Command,
		"task_id":     task.ID,
		"retry_count": task.RetryCount,
		"max_retries": task.MaxRetries,
	})

	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		log = log.WithField("timeout_reason", "deadline_exceeded")
	}

	if task.RetryCount >= task.MaxRetries {
		log.Warn("Max retries exceeded, marking as failed")
		return q.updateTaskStatus(ctx, task.ID, TaskStatusFailed)
	}

	var delay time.Duration
	if limiter, exists := q.commandLimiters[task.Command]; exists {
		delay = limiter.Reserve().Delay()
	} else {
		delay = task.RetryDelay
	}

	nextAttempt := time.Now().Add(delay)
	_, err := q.db.ExecWithRetry(ctx, `
		UPDATE tasks 
		SET status = ?, retry_count = retry_count + 1, next_attempt = ?
		WHERE id = ?
	`, TaskStatusPending, nextAttempt, task.ID)
	if err != nil {
		log.WithError(err).Error("Failed to reschedule task")
		return err
	}

	log.WithField("next_attempt", nextAttempt).Info("Task rescheduled")
	return nil
}

func (q *Queue) processCommandQueue(ctx context.Context, command string, handler commands.Command) {
	sem := q.commandSemaphores[command]
	limiter := q.commandLimiters[command]

	for range cap(sem) {
		go q.taskWorker(ctx, command, handler, sem, limiter)
	}

	<-ctx.Done()
}

func (q *Queue) taskWorker(ctx context.Context, command string, h commands.Command, sem chan struct{}, lim *rate.Limiter) {
	log := q.logger.WithField("command", command)
	log.Debug("Worker started")
	defer func() {
		log.Debug("Worker stopped")
		if r := recover(); r != nil {
			log.Error(fmt.Sprintf("recovered from panic: %v", r))
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case sem <- struct{}{}:
			task, err := q.lockAndGetTask(ctx, command)
			<-sem

			if err != nil {
				log.WithError(err).Error("Failed to get task")
				continue
			}
			if task == nil {
				log.Trace("No tasks available")
				time.Sleep(1 * time.Second)
				continue
			}

			reserve := lim.Reserve()
			if delay := reserve.Delay(); delay > 0 {
				log.WithFields(logger.Fields{
					"task":     task.ID,
					"wait_for": delay.String(),
				}).Debug("Rate limiting - delaying task")

				select {
				case <-time.After(delay):
					// continue processing
				case <-ctx.Done():
					reserve.Cancel()
					log.Debug("Cancelled due to context")
					return
				}
			}

			if err := q.handleTask(ctx, *task, h); err != nil {
				log.WithError(err).WithField("task_id", task.ID).Error("Task processing failed")
			}
		}
	}
}

func (q *Queue) lockAndGetTask(ctx context.Context, command string) (*Task, error) {
	var task Task
	err := q.db.GetDB().QueryRowContext(ctx, `
        UPDATE tasks 
        SET status = ?, last_attempt = ?
        WHERE id = (
            SELECT id FROM tasks 
            WHERE command = ? AND status = ? AND next_attempt <= ?
            ORDER BY id ASC 
            LIMIT 1
        )
        RETURNING id, command, update_data, retry_count, max_retries, retry_delay`,
		TaskStatusRunning, time.Now(), command, TaskStatusPending, time.Now(),
	).Scan(
		&task.ID, &task.Command, &task.UpdateData,
		&task.RetryCount, &task.MaxRetries, &task.RetryDelay,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	return &task, nil
}

func (q *Queue) updateTaskStatus(ctx context.Context, taskID int64, status TaskStatus) error {
	q.logger.WithFields(logger.Fields{
		"task_id": taskID,
	}).Info("Marking task as " + status)

	_, err := q.db.ExecWithRetry(ctx,
		"UPDATE tasks SET status = ? WHERE id = ?",
		status, taskID)
	return err
}

func (q *Queue) handleTask(ctx context.Context, task Task, handler commands.Command) error {
	cfg := handler.GetQueueConfig()
	timeout := cfg.Timeout
	deadline := time.Now().Add(timeout)

	log := q.logger.WithFields(logger.Fields{
		"command": task.Command,
		"task_id": task.ID,
	})
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	log.WithField("timeout", timeout.String()).Debug("Start processing task")
	start := time.Now()
	defer func() {
		log.WithFields(logger.Fields{
			"duration":        time.Since(start).String(),
			"status":          task.Status,
			"missed_deadline": time.Now().After(deadline),
		}).Debug("Task processing completed")
	}()

	q.logger.WithFields(logger.Fields{
		"task_id": task.ID,
		"command": task.Command,
		"update":  string(task.UpdateData),
	}).Debug("Unmarshalling task data")

	var err error
	if task.Update, err = task.GetUpdate(); err != nil {
		q.logger.Error(err.Error())
		q.updateTaskStatus(ctx, task.ID, TaskStatusFailed)
	}

	log.WithField("state", TaskStatusRunning).Info("Processing task")

	_, err = q.db.ExecWithRetry(ctx, "UPDATE tasks SET status = ?, last_attempt = ? WHERE id = ?",
		TaskStatusRunning, time.Now(), task.ID)
	if err != nil {
		return fmt.Errorf("failed to update task status: %w", err)
	}

	resultCh := make(chan error, 1)
	go func() {
		resultCh <- handler.Execute(*task.Update)
	}()

	select {
	case err := <-resultCh:
		if err != nil {
			if strings.Contains(err.Error(), "Status Code 400") &&
				strings.Contains(err.Error(), "Media not found") {
				log.WithError(err).Warn("Instagram media not found, marking as completed")
			} else {
				log.WithError(err).Error("Handler execution failed")
				return q.handleTaskError(ctx, task)
			}
		}
	case <-ctx.Done():
		log.WithFields(logger.Fields{
			"actual_duration": time.Since(start).String(),
			"retry_count":     task.RetryCount,
		}).Warn("Execution timeout exceeded")
		return q.handleTaskError(ctx, task)
	}

	// Mark as complete
	if err := q.updateTaskStatus(ctx, task.ID, TaskStatusComplete); err != nil {
		return fmt.Errorf("failed to mark task as complete: %w", err)
	}

	log.Info("Task completed successfully")
	return nil
}
