package referrals

import "time"

type AcceptRequest struct {
	InviteUsername string `json:"inviteUsername"`
}

type AcceptResponse struct {
	Accepted            bool   `json:"accepted"`
	AlreadyAccepted     bool   `json:"alreadyAccepted"`
	ReferrerUsername    string `json:"referrerUsername"`
	ReferrerDisplayName string `json:"referrerDisplayName"`
	RefereePoints       int    `json:"refereePoints"`
	ReferrerPoints      int    `json:"referrerPoints"`
}

type AcceptInput struct {
	RefereeUserID  string
	InviteUsername string
}

type AcceptOutcome struct {
	Created             bool
	ReferrerUserID      string
	ReferrerUsername    string
	ReferrerDisplayName string
	RefereePoints       int
	ReferrerPoints      int
	CreatedAt           time.Time
}
