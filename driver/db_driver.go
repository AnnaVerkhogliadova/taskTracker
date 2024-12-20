package driver

import (
	"context"
	"errors"
	"fmt"
	"github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"strings"
	internal_error "taskTracker/errors"
	"taskTracker/model"
)

type dbDriver struct {
	rwdb *pgxpool.Pool
	rdb  *pgxpool.Pool
	qb   *squirrel.StatementBuilderType
}

func NewDbDriver(rwdb *pgxpool.Pool, rdb *pgxpool.Pool) (ITasks, error) {
	qb := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	return &dbDriver{
		rwdb: rwdb,
		rdb:  rdb,
		qb:   &qb,
	}, nil
}

func (d *dbDriver) Create(ctx context.Context, task *model.Task) (*model.Task, error) {
	tx, err := d.rwdb.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("error creating task: %w", err)
	}

	var taskId uint64

	err = tx.QueryRow(ctx, queryCreateTask,
		task.Title, task.Description, task.Status,
	).Scan(&taskId)

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			if strings.Contains(pgErr.Detail, "application_name") && pgErr.Code == "23505" {
				return nil, internal_error.NewErrTitleTaskAlreadyExist(task.Title)
			} else if pgErr.Code == "23505" {
				return nil, internal_error.NewErrTitleTaskAlreadyExist(task.Title)
			}
		}

		return nil, fmt.Errorf("error application creating: %w", err)
	}

	task = &model.Task{
		ID:          taskId,
		Title:       task.Title,
		Description: task.Description,
		Status:      task.Status,
		CreatedAt:   task.CreatedAt}

	if err != nil {
		zerolog.Ctx(ctx).Error().Err(err).Msg("error executing query")
		tx.Rollback(ctx)
		return nil, fmt.Errorf("error creating task in db: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		tx.Rollback(ctx)
		return nil, fmt.Errorf("error committing transaction: %w", err)
	}

	return task, nil
}

func (d *dbDriver) SetStatus(ctx context.Context, taskId uint64, status *uint64) error {
	row, err := d.rwdb.Query(
		ctx,
		querySetStatus,
		taskId, status)

	defer row.Close()
	if err != nil {
		return fmt.Errorf("error set status: %w", err)
	}

	return nil
}

func (d *dbDriver) SetSubTaskStatus(ctx context.Context, subTaskId uint64, status *uint64) error {
	row, err := d.rwdb.Query(
		ctx,
		querySubTaskSetStatus,
		subTaskId, status)

	defer row.Close()
	if err != nil {
		return fmt.Errorf("error set sub task status: %w", err)
	}

	return nil
}

func (d *dbDriver) Get(ctx context.Context, taskId uint64) (*model.Task, error) {
	tx, err := d.rwdb.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("error creating task: %w", err)
	}

	row, err := tx.Query(
		ctx,
		queryGet,
		taskId)

	if err != nil {
		return nil, fmt.Errorf("error Get in db: %w", err)
	}

	results, err := pgx.CollectRows(row, pgx.RowToStructByName[model.Task])
	if err != nil {
		return nil, fmt.Errorf("errorCollectRows for Get: %w", err)
	}

	if len(results) == 0 {
		return nil, internal_error.NewErrTaskNotFound(taskId)
	}

	return &results[0], nil
}

func (d *dbDriver) Delete(ctx context.Context, taskId uint64) error {
	row, err := d.rwdb.Query(
		ctx,
		queryDelete,
		taskId)

	defer row.Close()
	if err != nil {
		return fmt.Errorf("error deleting Task: %w", err)
	}

	return nil
}

func (d *dbDriver) DeleteSubTask(ctx context.Context, subTaskId uint64) error {
	row, err := d.rwdb.Query(
		ctx,
		queryDeleteSubTask,
		subTaskId)

	defer row.Close()
	if err != nil {
		return fmt.Errorf("error deleting sub task: %w", err)
	}

	return nil
}

func (d *dbDriver) GetList(ctx context.Context, status *uint64) ([]*model.Task, error) {
	row, err := d.rwdb.Query(
		ctx,
		queryGetList,
		status)

	if err != nil {
		return nil, fmt.Errorf("error Get in db: %w", err)
	}

	results, err := pgx.CollectRows(row, pgx.RowToStructByName[model.Task])
	if err != nil {
		return nil, fmt.Errorf("errorCollectRows for GetList: %w", err)
	}

	tasks := make([]*model.Task, len(results))
	for i := range results {
		task := results[i]
		tasks[i] = &task
	}

	return tasks, nil
}

func (d *dbDriver) CreateSubTask(ctx context.Context, subTask *model.SubTask) (*model.SubTask, error) {
	tx, err := d.rwdb.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("error creating task: %w", err)
	}

	var exists bool
	err = tx.QueryRow(ctx, queryExistTaskId, subTask.TaskID).Scan(&exists)
	if err != nil {
		return nil, nil
	}

	if !exists {
		tx.Rollback(ctx)
		return nil, internal_error.NewErrTaskNotFound(subTask.TaskID)
	}

	var subTaskId uint64
	err = tx.QueryRow(ctx, queryCreateSubTask,
		subTask.TaskID,
		subTask.Title,
		subTask.Description,
		subTask.Status,
	).Scan(&subTaskId)

	subTask = &model.SubTask{
		ID:          subTaskId,
		TaskID:      subTask.TaskID,
		Title:       subTask.Title,
		Description: subTask.Description,
		Status:      subTask.Status,
		CreatedAt:   subTask.CreatedAt}

	if err != nil {
		zerolog.Ctx(ctx).Error().Err(err).Msg("error executing query")
		tx.Rollback(ctx)
		return nil, fmt.Errorf("error creating task in db: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		tx.Rollback(ctx)
		return nil, fmt.Errorf("error committing transaction: %w", err)
	}

	return subTask, nil
}

//func WithPGTransaction(ctx context.Context, db *pgxpool.Pool, fn func(tx pgx.Tx) error) error {
//	tx, err := db.Begin(ctx)
//	if err != nil {
//		return err
//	}
//
//	if err := fn(tx); err != nil {
//		_ = tx.Rollback(ctx)
//		return err
//	}
//
//	return tx.Commit(ctx)
//}
