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
