package submissions

import "context"

type Repository interface {
	Create(ctx context.Context, submission *Submission) error
}
