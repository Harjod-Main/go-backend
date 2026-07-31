package mediaurl

import "testing"

func TestValidMediaURLs(t *testing.T) {
	t.Parallel()

	ResetForTest()
	Configure("https://sycwdwymeirxowbrqdgd.supabase.co")
	t.Cleanup(ResetForTest)

	prefix := PublicPrefix()
	good := prefix + "11111111-1111-1111-1111-111111111111/reviews/a.jpg"

	if !ValidMediaURLs(nil, MaxURLLen) {
		t.Fatal("nil slice should be valid")
	}
	if !ValidMediaURLs([]string{}, MaxURLLen) {
		t.Fatal("empty slice should be valid")
	}
	if !ValidMediaURLs([]string{good}, MaxURLLen) {
		t.Fatal("media bucket URL should be valid")
	}
	if ValidMediaURLs([]string{"https://example.com/a.jpg"}, MaxURLLen) {
		t.Fatal("foreign host should be rejected")
	}
	if ValidMediaURLs([]string{"file:///tmp/a.jpg"}, MaxURLLen) {
		t.Fatal("file URI should be rejected")
	}
	if ValidMediaURLs([]string{prefix + "../etc/passwd"}, MaxURLLen) {
		t.Fatal("path traversal should be rejected")
	}
	if ValidMediaURLs([]string{good + "?x=1"}, MaxURLLen) {
		t.Fatal("query string should be rejected")
	}
}

func TestValidMediaURLsFailClosedWithoutConfigure(t *testing.T) {
	t.Parallel()

	ResetForTest()
	t.Cleanup(ResetForTest)

	if ValidMediaURLs([]string{"https://sycwdwymeirxowbrqdgd.supabase.co/storage/v1/object/public/media/a.jpg"}, MaxURLLen) {
		t.Fatal("should reject when not configured")
	}
	if !ValidMediaURLs(nil, MaxURLLen) {
		t.Fatal("empty should still pass when not configured")
	}
}

func TestValidAvatarValue(t *testing.T) {
	t.Parallel()

	ResetForTest()
	Configure("https://sycwdwymeirxowbrqdgd.supabase.co")
	t.Cleanup(ResetForTest)

	if !ValidAvatarValue("preset:0") {
		t.Fatal("preset key should be valid")
	}
	good := PublicPrefix() + "u/avatars/a.png"
	if !ValidAvatarValue(good) {
		t.Fatal("media avatar should be valid")
	}
	if ValidAvatarValue("https://cdn.example.com/a.png") {
		t.Fatal("foreign avatar should be rejected")
	}
	if ValidAvatarValue("file:///avatar.png") {
		t.Fatal("file avatar should be rejected")
	}
}

func TestValidPrivateReportPaths(t *testing.T) {
	t.Parallel()

	good := "11111111-1111-1111-1111-111111111111/reports/a.jpg"

	if !ValidPrivateReportPaths(nil, MaxURLLen) {
		t.Fatal("nil slice should be valid")
	}
	if !ValidPrivateReportPaths([]string{}, MaxURLLen) {
		t.Fatal("empty slice should be valid")
	}
	if !ValidPrivateReportPaths([]string{good}, MaxURLLen) {
		t.Fatal("private report path should be valid")
	}
	if ValidPrivateReportPaths([]string{"https://example.com/a.jpg"}, MaxURLLen) {
		t.Fatal("public URL should be rejected for private report paths")
	}
	if ValidPrivateReportPaths([]string{"/reports/a.jpg"}, MaxURLLen) {
		t.Fatal("absolute path should be rejected")
	}
	if ValidPrivateReportPaths([]string{"111/reports/../a.jpg"}, MaxURLLen) {
		t.Fatal("path traversal should be rejected")
	}
	if ValidPrivateReportPaths([]string{"111/avatar/a.jpg"}, MaxURLLen) {
		t.Fatal("wrong folder should be rejected")
	}
}

func TestValidOwnedPrivateReportPaths(t *testing.T) {
	t.Parallel()

	owner := "11111111-1111-1111-1111-111111111111"
	other := "22222222-2222-2222-2222-222222222222"
	owned := owner + "/reports/a.jpg"
	foreign := other + "/reports/private.jpg"

	if !ValidOwnedPrivateReportPaths(nil, owner, MaxURLLen) {
		t.Fatal("nil slice should be valid")
	}
	if !ValidOwnedPrivateReportPaths([]string{}, owner, MaxURLLen) {
		t.Fatal("empty slice should be valid")
	}
	if !ValidOwnedPrivateReportPaths([]string{owned}, owner, MaxURLLen) {
		t.Fatal("owned path should be valid")
	}
	if ValidOwnedPrivateReportPaths([]string{foreign}, owner, MaxURLLen) {
		t.Fatal("other user's path should be rejected")
	}
	if ValidOwnedPrivateReportPaths([]string{owned}, "", MaxURLLen) {
		t.Fatal("empty owner should reject non-empty paths")
	}
}
