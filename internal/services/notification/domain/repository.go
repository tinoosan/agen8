package domain

import "context"

// Repository persists notifications. The reconciler drives most of these:
// Insert for new derived specs, Dismiss to retire standing nudges whose
// condition cleared, and ExistsByThrottleKey to enforce "tell the user once"
// for one-time events even across dismissals.
type Repository interface {
	// Insert stores a new notification. ID is expected to be set by the caller.
	Insert(ctx context.Context, n Notification) error

	// ListActive returns undismissed notifications for a user within a project,
	// newest first. Read-but-undismissed rows are included (they still show in
	// the inbox, just not counted as unread).
	ListActive(ctx context.Context, userID, projectID string) ([]Notification, error)

	// ExistsByThrottleKey reports whether any notification (active, read, or
	// dismissed) was ever stored for this user/project/throttle key. Used to
	// keep one-time events from resurrecting after dismissal.
	ExistsByThrottleKey(ctx context.Context, userID, projectID, throttleKey string) (bool, error)

	// MarkRead stamps read_at on a single notification owned by the user.
	MarkRead(ctx context.Context, userID, id string) error

	// MarkAllRead stamps read_at on every unread, undismissed notification for
	// the user within the project. Returns the number of rows affected.
	MarkAllRead(ctx context.Context, userID, projectID string) (int, error)

	// Dismiss stamps dismissed_at on a single notification owned by the user.
	Dismiss(ctx context.Context, userID, id string) error
}
