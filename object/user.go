package object

import (
	"fmt"
	"strings"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/casosorg/casos/conf"
	"github.com/casosorg/casos/util"
	"golang.org/x/crypto/bcrypt"
)

const (
	UserOwner              = "basic"
	maxPasswordLengthBytes = 72
	// Compared against when the requested user does not exist, so that a wrong
	// username costs the same time as a wrong password.
	unknownUserPasswordHash = "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"

	DefaultAdminName     = "admin"
	DefaultAdminPassword = "123"
)

// User is the built-in account used when CasOS runs without Casdoor.
type User struct {
	Owner       string `xorm:"varchar(100) notnull pk" json:"owner"`
	Name        string `xorm:"varchar(100) notnull pk" json:"name"`
	CreatedTime string `xorm:"varchar(100)" json:"createdTime"`
	UpdatedTime string `xorm:"varchar(100)" json:"updatedTime"`

	RuntimeName string `xorm:"varchar(100) index" json:"runtimeName"`
	DisplayName string `xorm:"varchar(100)" json:"displayName"`
	Avatar      string `xorm:"text" json:"avatar"`
	Homepage    string `xorm:"varchar(100)" json:"homepage"`

	PasswordHash string `xorm:"varchar(150)" json:"-"`
	IsForbidden  bool   `json:"isForbidden"`
	IsDeleted    bool   `json:"isDeleted"`
}

// IsSigninEnabled reports whether the built-in password sign-in is in use.
// Casdoor takes over as soon as an endpoint is configured.
func IsSigninEnabled() bool {
	return conf.GetConfigString("casdoorEndpoint") == ""
}

func GetRuntimeName(name string) string {
	return strings.TrimSpace(name)
}

func normalizeUserName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("username cannot be empty")
	}
	if strings.Contains(name, "/") {
		return "", fmt.Errorf("username cannot contain slash")
	}
	if len(name) > 100 {
		return "", fmt.Errorf("username is too long")
	}
	return name, nil
}

func getPasswordHash(password string) (string, error) {
	if password == "" {
		return "", fmt.Errorf("password cannot be empty")
	}
	if len([]byte(password)) > maxPasswordLengthBytes {
		return "", fmt.Errorf("password cannot be longer than %d bytes", maxPasswordLengthBytes)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func (user *User) GetId() string {
	return fmt.Sprintf("%s/%s", user.Owner, user.Name)
}

func (user *User) ToCasdoorUser() casdoorsdk.User {
	displayName := user.DisplayName
	if displayName == "" {
		displayName = user.Name
	}

	return casdoorsdk.User{
		Owner:       UserOwner,
		Name:        user.RuntimeName,
		CreatedTime: user.CreatedTime,
		UpdatedTime: user.UpdatedTime,
		Id:          user.RuntimeName,
		IsAdmin:     true,
		DisplayName: displayName,
		Avatar:      user.Avatar,
		Homepage:    user.Homepage,
		IsForbidden: user.IsForbidden,
		IsDeleted:   user.IsDeleted,
	}
}

func GetUser(name string) (*User, error) {
	name, err := normalizeUserName(name)
	if err != nil {
		return nil, err
	}

	user := User{Owner: UserOwner, Name: name}
	existed, err := ormer.Engine.Get(&user)
	if err != nil {
		return nil, err
	}
	if !existed {
		return nil, nil
	}
	return &user, nil
}

func GetUserByRuntimeName(runtimeName string) (*User, error) {
	if runtimeName == "" {
		return nil, nil
	}

	user := User{Owner: UserOwner, RuntimeName: runtimeName}
	existed, err := ormer.Engine.Get(&user)
	if err != nil {
		return nil, err
	}
	if !existed {
		return nil, nil
	}
	return &user, nil
}

func GetUserList() ([]*User, error) {
	users := []*User{}
	err := ormer.Engine.Asc("name").Find(&users, &User{Owner: UserOwner})
	if err != nil {
		return users, err
	}
	return users, nil
}

func AddUser(user *User, password string) (bool, error) {
	name, err := normalizeUserName(user.Name)
	if err != nil {
		return false, err
	}

	user.Owner = UserOwner
	user.Name = name
	user.RuntimeName = GetRuntimeName(name)
	user.CreatedTime = util.GetCurrentTime()
	user.UpdatedTime = user.CreatedTime
	user.PasswordHash, err = getPasswordHash(password)
	if err != nil {
		return false, err
	}

	affected, err := ormer.Engine.Insert(user)
	if err != nil {
		return false, err
	}
	return affected != 0, nil
}

func UpdateUserProfile(user *User) error {
	user.UpdatedTime = util.GetCurrentTime()
	_, err := ormer.Engine.Where("owner = ? AND name = ?", user.Owner, user.Name).
		Cols("display_name", "avatar", "updated_time").Update(user)
	return err
}

func UpdateUserPassword(user *User, password string) error {
	passwordHash, err := getPasswordHash(password)
	if err != nil {
		return err
	}

	user.PasswordHash = passwordHash
	user.UpdatedTime = util.GetCurrentTime()
	_, err = ormer.Engine.Where("owner = ? AND name = ?", user.Owner, user.Name).
		Cols("password_hash", "updated_time").Update(user)
	return err
}

func CheckUserPassword(user *User, password string) bool {
	if user == nil || len([]byte(password)) > maxPasswordLengthBytes {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) == nil
}

func compareUnknownUserPassword(password string) {
	if len([]byte(password)) <= maxPasswordLengthBytes {
		_ = bcrypt.CompareHashAndPassword([]byte(unknownUserPasswordHash), []byte(password))
	}
}

func VerifyUser(username string, password string) (*User, bool, error) {
	user, err := GetUser(username)
	if err != nil {
		compareUnknownUserPassword(password)
		return nil, false, err
	}
	if user == nil || user.IsDeleted || user.IsForbidden || user.PasswordHash == "" {
		compareUnknownUserPassword(password)
		return nil, false, nil
	}
	if len([]byte(password)) > maxPasswordLengthBytes || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		return nil, false, nil
	}

	return user, true, nil
}

// IsAdminUsingDefaultPassword reports whether the built-in admin still carries
// the password it was created with. While that is true CasOS keeps the first-run
// experience friction-free; changing the password ends it.
func IsAdminUsingDefaultPassword() bool {
	user, err := GetUser(DefaultAdminName)
	if err != nil || user == nil {
		return false
	}
	return CheckUserPassword(user, DefaultAdminPassword)
}

func InitUsers() {
	if !IsSigninEnabled() {
		return
	}

	user, err := GetUser(DefaultAdminName)
	if err != nil {
		panic(err)
	}
	if user != nil {
		return
	}

	_, err = AddUser(&User{
		Name:        DefaultAdminName,
		DisplayName: "Admin",
	}, DefaultAdminPassword)
	if err != nil {
		panic(err)
	}
}
