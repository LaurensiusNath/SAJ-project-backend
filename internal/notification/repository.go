package notification

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/nathan/cnc-pm-backend/internal/pgconv"
	"github.com/nathan/cnc-pm-backend/internal/repository/sqlcgen"
)

type Repository interface {
	Create(ctx context.Context, n Notification) (Notification, error)
	// ExistsSentToday dipakai job.ReminderService sebagai idempotency check -
	// lihat db/queries/notifications.sql untuk alasannya.
	ExistsSentToday(ctx context.Context, jobID uuid.UUID, subject string) (bool, error)
}

type sqlcRepository struct {
	q *sqlcgen.Queries
}

func NewRepository(q *sqlcgen.Queries) Repository {
	return &sqlcRepository{q: q}
}

func (r *sqlcRepository) Create(ctx context.Context, n Notification) (Notification, error) {
	row, err := r.q.CreateNotification(ctx, sqlcgen.CreateNotificationParams{
		Channel:      string(n.Channel),
		Recipient:    n.Recipient,
		Subject:      pgconv.ToText(n.Subject),
		Message:      n.Message,
		Status:       string(n.Status),
		ErrorMessage: pgconv.ToText(n.ErrorMessage),
		JobID:        pgconv.ToNullableUUID(n.JobID),
		InvoiceID:    pgconv.ToNullableUUID(n.InvoiceID),
	})
	if err != nil {
		return Notification{}, fmt.Errorf("insert notification log: %w", err)
	}
	return fromRow(row), nil
}

func (r *sqlcRepository) ExistsSentToday(ctx context.Context, jobID uuid.UUID, subject string) (bool, error) {
	exists, err := r.q.ExistsNotificationSentToday(ctx, sqlcgen.ExistsNotificationSentTodayParams{
		JobID:   pgconv.ToUUID(jobID),
		Subject: pgconv.ToText(&subject),
	})
	if err != nil {
		return false, fmt.Errorf("check notification sent today: %w", err)
	}
	return exists, nil
}

func fromRow(row sqlcgen.Notification) Notification {
	return Notification{
		ID:           pgconv.FromUUID(row.ID),
		Channel:      Channel(row.Channel),
		Recipient:    row.Recipient,
		Subject:      pgconv.FromText(row.Subject),
		Message:      row.Message,
		Status:       Status(row.Status),
		ErrorMessage: pgconv.FromText(row.ErrorMessage),
		JobID:        pgconv.FromNullableUUID(row.JobID),
		InvoiceID:    pgconv.FromNullableUUID(row.InvoiceID),
		CreatedAt:    pgconv.FromTimestamptz(row.CreatedAt),
	}
}
