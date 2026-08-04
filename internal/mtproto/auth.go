package mtproto

import (
	"context"
	"fmt"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/telegram/auth/qrlogin"
	"github.com/gotd/td/tg"
)

// CodeFunc is called to solicit a verification code from the user.
type CodeFunc func(ctx context.Context, phone string) (string, error)

// PasswordFunc is called to solicit the 2FA cloud password.
type PasswordFunc func(ctx context.Context) (string, error)

// QRFunc is called to display the QR URL to the user.
type QRFunc func(ctx context.Context, url string) error

// phoneFlowAdapter adapts a phone number plus code/password callbacks into the
// auth.UserAuthenticator gotd's auth.Flow drives. Account sign-up is rejected
// (the same no-sign-up stance auth.Constant takes). The key point: gotd's Flow
// invokes Password() only when Telegram reports the account needs a 2FA
// password, so pwdFn (and the CLI's --password flag) is actually reached —
// the previous explicit `needsPassword(err)` post-check could never fire
// because err was already consumed by the Status() call above it.
type phoneFlowAdapter struct {
	phone string
	code  CodeFunc
	pwd   PasswordFunc
}

func (a phoneFlowAdapter) Phone(context.Context) (string, error) { return a.phone, nil }
func (a phoneFlowAdapter) Code(ctx context.Context, _ *tg.AuthSentCode) (string, error) {
	return a.code(ctx, a.phone)
}
func (a phoneFlowAdapter) Password(ctx context.Context) (string, error) { return a.pwd(ctx) }
func (a phoneFlowAdapter) SignUp(context.Context) (auth.UserInfo, error) {
	return auth.UserInfo{}, fmt.Errorf("telegram-cli does not support creating new accounts")
}
func (a phoneFlowAdapter) AcceptTermsOfService(ctx context.Context, tos tg.HelpTermsOfService) error {
	return &auth.SignUpRequired{TermsOfService: tos}
}

// LoginPhone runs the phone + code + optional 2FA flow for the given phone number.
func LoginPhone(ctx context.Context, client *telegram.Client, phone string, codeFn CodeFunc, pwdFn PasswordFunc) error {
	ac := client.Auth()
	status, err := ac.Status(ctx)
	if err != nil {
		return fmt.Errorf("auth status: %w", err)
	}
	if status.Authorized {
		return fmt.Errorf("account is already logged in as %s", status.User.Username)
	}

	f := auth.NewFlow(
		phoneFlowAdapter{phone: phone, code: codeFn, pwd: pwdFn},
		auth.SendCodeOptions{},
	)
	if err := ac.IfNecessary(ctx, f); err != nil {
		return fmt.Errorf("auth flow: %w", err)
	}
	return nil
}

// LoginQR runs the QR-code login flow. showFn renders the QR token; loggedIn is
// populated when the user scans the code on their phone.
//
// The client passed in must have been created with NoUpdates disabled and an
// update dispatcher attached (see Manager.DialAndRunUncheckedWithUpdates): the
// UpdateLoginToken update is what unblocks the flow the moment the user scans,
// instead of waiting for token expiry. Callers pass the dispatcher they
// configured the client with.
func LoginQR(ctx context.Context, client *telegram.Client, dispatcher tg.UpdateDispatcher, showFn QRFunc) error {
	ac := client.Auth()
	status, err := ac.Status(ctx)
	if err != nil {
		return fmt.Errorf("auth status: %w", err)
	}
	if status.Authorized {
		return fmt.Errorf("account is already logged in as %s", status.User.Username)
	}

	qr := client.QR()
	loggedIn := qrlogin.OnLoginToken(dispatcher)
	_, err = qr.Auth(ctx,
		loggedIn,
		func(ctx context.Context, token qrlogin.Token) error {
			return showFn(ctx, token.URL())
		},
	)
	if err != nil {
		return fmt.Errorf("QR login: %w", err)
	}
	return nil
}

// Logout performs auth.LogOut and returns any error.
func LogoutAuth(ctx context.Context, client *telegram.Client) error {
	_, err := client.API().AuthLogOut(ctx)
	return err
}
