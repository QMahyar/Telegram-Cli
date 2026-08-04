package mtproto

import (
	"context"
	"fmt"
	"strings"

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
		auth.Constant(phone, "", auth.CodeAuthenticatorFunc(func(ctx context.Context, sentCode *tg.AuthSentCode) (string, error) {
			return codeFn(ctx, phone)
		})),
		auth.SendCodeOptions{},
	)
	if err := ac.IfNecessary(ctx, f); err != nil {
		return fmt.Errorf("auth flow: %w", err)
	}

	// Check if 2FA is needed.
	status2, err := ac.Status(ctx)
	if err != nil {
		return fmt.Errorf("auth status after code: %w", err)
	}
	if !status2.Authorized && needsPassword(err) {
		pwd, pwdErr := pwdFn(ctx)
		if pwdErr != nil {
			return fmt.Errorf("2FA password: %w", pwdErr)
		}
		if _, err := ac.Password(ctx, pwd); err != nil {
			return fmt.Errorf("2FA login: %w", err)
		}
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

// needsPassword checks if the error signals a 2FA password requirement.
func needsPassword(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "SESSION_PASSWORD_NEEDED")
}

// Logout performs auth.LogOut and returns any error.
func LogoutAuth(ctx context.Context, client *telegram.Client) error {
	_, err := client.API().AuthLogOut(ctx)
	return err
}
