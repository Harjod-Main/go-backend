package submissions

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ContributeInput publishes one community privilege onto an existing place.
type ContributeInput struct {
	PlaceID       string
	ParkingAreaID string
	Latitude      float64
	Longitude     float64
	UserID        *string
	Kind          string // stamp | reserve | ev
	Value         json.RawMessage
}

// ContributePrivilege inserts a single stamp/reserve/EV privilege for an existing place.
func ContributePrivilege(ctx context.Context, pool *pgxpool.Pool, in ContributeInput) error {
	wrapped, err := wrapPrivilegeEntry(in.Value)
	if err != nil {
		return err
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin contribute privilege: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := contributePrivilegeTx(ctx, tx, in, wrapped); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit contribute privilege: %w", err)
	}
	return nil
}

func contributePrivilegeTx(
	ctx context.Context,
	tx pgx.Tx,
	in ContributeInput,
	wrapped json.RawMessage,
) error {
	switch in.Kind {
	case "stamp":
		stamps := parseStampEntries(wrapped)
		if len(stamps) == 0 {
			return fmt.Errorf("invalid stamp payload")
		}
		return publishStamps(ctx, tx, in.PlaceID, in.UserID, stamps)
	case "reserve":
		items := parseReservedEntries(wrapped)
		if len(items) == 0 {
			return fmt.Errorf("invalid reserved payload")
		}
		return publishReserved(ctx, tx, in.ParkingAreaID, in.UserID, items)
	case "ev":
		items := parseEVEntries(wrapped)
		if len(items) == 0 {
			return fmt.Errorf("invalid ev payload")
		}
		if err := publishEVCharges(ctx, tx, in.PlaceID, in.ParkingAreaID, in.Latitude, in.Longitude, in.UserID, items); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, markParkingAreaHasEVSQL, in.ParkingAreaID); err != nil {
			return fmt.Errorf("mark parking area has EV: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("unsupported privilege kind %q", in.Kind)
	}
}

func wrapPrivilegeEntry(value json.RawMessage) (json.RawMessage, error) {
	if len(value) == 0 || string(value) == "null" {
		return nil, fmt.Errorf("missing privilege value")
	}
	entry := struct {
		ID    string          `json:"id"`
		Value json.RawMessage `json:"value"`
	}{
		ID:    "contribute",
		Value: value,
	}
	return json.Marshal([]any{entry})
}
