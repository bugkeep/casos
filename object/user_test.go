package object

import (
	"path/filepath"
	"testing"
)

// withTestOrmer points the package-level ormer at a throwaway SQLite database so
// the user helpers can be exercised without a real installation.
func withTestOrmer(t *testing.T) {
	t.Helper()

	previous := ormer
	adapter := NewAdapter("sqlite", filepath.Join(t.TempDir(), "casos.db"), "")
	adapter.createTable()
	ormer = adapter
	t.Cleanup(func() {
		ormer = previous
		adapter.close()
	})
}

func addTestUser(t *testing.T, name string, password string) *User {
	t.Helper()

	added, err := AddUser(&User{Name: name, DisplayName: "Admin"}, password)
	if err != nil {
		t.Fatalf("AddUser(%q): %v", name, err)
	}
	if !added {
		t.Fatalf("AddUser(%q) did not insert a row", name)
	}

	user, err := GetUser(name)
	if err != nil {
		t.Fatalf("GetUser(%q): %v", name, err)
	}
	if user == nil {
		t.Fatalf("GetUser(%q) returned no user", name)
	}
	return user
}

func TestAddUserStoresOnlyAHash(t *testing.T) {
	withTestOrmer(t)

	user := addTestUser(t, "admin", "123")
	if user.PasswordHash == "123" || user.PasswordHash == "" {
		t.Fatalf("PasswordHash = %q, want a bcrypt hash", user.PasswordHash)
	}
	if !CheckUserPassword(user, "123") {
		t.Fatal("CheckUserPassword rejected the password it was created with")
	}
	if CheckUserPassword(user, "1234") {
		t.Fatal("CheckUserPassword accepted a wrong password")
	}
}

func TestVerifyUser(t *testing.T) {
	withTestOrmer(t)
	addTestUser(t, "admin", "123")

	tests := []struct {
		name     string
		username string
		password string
		wantOk   bool
		wantErr  bool
	}{
		{name: "correct credentials", username: "admin", password: "123", wantOk: true},
		{name: "wrong password", username: "admin", password: "124"},
		{name: "unknown user", username: "nobody", password: "123"},
		{name: "surrounding whitespace is trimmed", username: "  admin  ", password: "123", wantOk: true},
		{name: "empty username", username: "", password: "123", wantErr: true},
		// bcrypt silently truncates at 72 bytes, so anything longer is rejected
		// outright rather than being compared against a truncated copy.
		{name: "over-long password", username: "admin", password: string(make([]byte, maxPasswordLengthBytes+1))},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			user, ok, err := VerifyUser(test.username, test.password)
			if (err != nil) != test.wantErr {
				t.Fatalf("VerifyUser error = %v, wantErr = %v", err, test.wantErr)
			}
			if ok != test.wantOk {
				t.Fatalf("VerifyUser ok = %v, want %v", ok, test.wantOk)
			}
			if test.wantOk && user == nil {
				t.Fatal("VerifyUser reported success but returned no user")
			}
			if !test.wantOk && user != nil {
				t.Fatal("VerifyUser reported failure but returned a user")
			}
		})
	}
}

func TestVerifyUserRejectsDisabledAccounts(t *testing.T) {
	withTestOrmer(t)
	user := addTestUser(t, "admin", "123")

	for _, test := range []struct {
		name  string
		apply func(*User)
	}{
		{name: "forbidden", apply: func(u *User) { u.IsForbidden = true }},
		{name: "deleted", apply: func(u *User) { u.IsDeleted = true }},
	} {
		t.Run(test.name, func(t *testing.T) {
			flagged := *user
			test.apply(&flagged)
			if _, err := ormer.Engine.Where("owner = ? AND name = ?", flagged.Owner, flagged.Name).
				Cols("is_forbidden", "is_deleted").Update(&flagged); err != nil {
				t.Fatalf("update user flags: %v", err)
			}
			t.Cleanup(func() {
				if _, err := ormer.Engine.Where("owner = ? AND name = ?", user.Owner, user.Name).
					Cols("is_forbidden", "is_deleted").Update(user); err != nil {
					t.Fatalf("restore user flags: %v", err)
				}
			})

			if _, ok, err := VerifyUser("admin", "123"); err != nil || ok {
				t.Fatalf("VerifyUser ok = %v, err = %v, want ok = false", ok, err)
			}
		})
	}
}

func TestUpdateUserPasswordEndsTheDefaultPassword(t *testing.T) {
	withTestOrmer(t)
	user := addTestUser(t, DefaultAdminName, DefaultAdminPassword)

	if !IsAdminUsingDefaultPassword() {
		t.Fatal("IsAdminUsingDefaultPassword = false right after the account was created")
	}

	if err := UpdateUserPassword(user, "a-much-better-password"); err != nil {
		t.Fatalf("UpdateUserPassword: %v", err)
	}

	if IsAdminUsingDefaultPassword() {
		t.Fatal("IsAdminUsingDefaultPassword = true after the password was changed")
	}
	if _, ok, err := VerifyUser(DefaultAdminName, DefaultAdminPassword); err != nil || ok {
		t.Fatalf("the old password still works: ok = %v, err = %v", ok, err)
	}
	if _, ok, err := VerifyUser(DefaultAdminName, "a-much-better-password"); err != nil || !ok {
		t.Fatalf("the new password does not work: ok = %v, err = %v", ok, err)
	}
}

func TestInitUsersIsIdempotent(t *testing.T) {
	withTestOrmer(t)
	t.Setenv("casdoorEndpoint", "")

	InitUsers()
	first, err := GetUser(DefaultAdminName)
	if err != nil || first == nil {
		t.Fatalf("GetUser after InitUsers: user = %v, err = %v", first, err)
	}

	// A second run must not reset a password the operator has already changed.
	if err = UpdateUserPassword(first, "a-much-better-password"); err != nil {
		t.Fatalf("UpdateUserPassword: %v", err)
	}
	InitUsers()

	if IsAdminUsingDefaultPassword() {
		t.Fatal("InitUsers put the default password back")
	}
}

func TestInitUsersSkippedWhenCasdoorIsConfigured(t *testing.T) {
	withTestOrmer(t)
	t.Setenv("casdoorEndpoint", "https://door.example.com")

	if IsSigninEnabled() {
		t.Fatal("IsSigninEnabled = true while Casdoor is configured")
	}

	InitUsers()

	user, err := GetUser(DefaultAdminName)
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if user != nil {
		t.Fatal("InitUsers created a built-in account even though Casdoor is configured")
	}
}

func TestGetUserRejectsMalformedNames(t *testing.T) {
	withTestOrmer(t)

	for _, name := range []string{"", "   ", "a/b"} {
		if _, err := GetUser(name); err == nil {
			t.Fatalf("GetUser(%q) returned no error", name)
		}
	}
}
