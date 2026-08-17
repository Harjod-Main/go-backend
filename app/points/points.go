package points

// Award amounts for gamification actions. Check-in keeps its own constants in
// the checkins package (50+50) because that flow stores a breakdown on the row.
const (
	ReviewCreate        = 50
	PlaceSubmission     = 50
	PrivilegeCorrection = 10
	ReferralReferrer    = 50
	ReferralReferee     = 50
)
