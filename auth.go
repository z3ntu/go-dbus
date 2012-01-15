package dbus

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
)

var (
	ErrAuthUnknownCommand = errors.New("UnknowAuthCommand")
	ErrAuthFailed         = errors.New("AuthenticationFailed")
)

type Authenticator interface {
	Mechanism() string
	Authenticate() string
}

type AuthExternal struct {
}

func (p *AuthExternal) Mechanism() string { return "EXTERNAL" }
func (p *AuthExternal) Authenticate() string {
	return fmt.Sprintf("%x", fmt.Sprintf("%d", os.Getuid()))
}

type authStatus int

const (
	statusStarting authStatus = iota
	statusWaitingForData
	statusWaitingForOk
	statusWaitingForReject
	statusAuthContinue
	statusAuthOk
	statusAuthError
	statusAuthenticated
	statusAuthNext
)

type authState struct {
	status authStatus
	auth   Authenticator
	conn   net.Conn
}

func (p *authState) _NextMessage() []string {
	b := make([]byte, 4096)
	p.conn.Read(b)
	retstr := string(b)
	return strings.SplitN(strings.Trim(retstr, " "), " ", -1)
}

func (p *authState) _Send(msg string) {
	p.conn.Write([]byte(msg + "\r\n"))
}

func (p *authState) Authenticate(conn net.Conn, mech Authenticator) error {
	p.auth = mech
	p.conn = conn
	p.status = statusStarting
	p._Send(strings.Join([]string{"AUTH", p.auth.Mechanism(), p.auth.Authenticate()}, " "))

	for p.status != statusAuthenticated {
		if nil == p.auth {
			return ErrAuthFailed
		}
		if err := p._NextState(); err != nil {
			return err
		}
	}
	return nil
}

func (p *authState) _NextState() (err error) {
	nextMsg := p._NextMessage()

	if statusStarting == p.status {
		switch nextMsg[0] {
		case "CONTINUE":
			p.status = statusWaitingForData
		case "OK":
			p.status = statusWaitingForOk
		}
	}

	switch p.status {
	case statusWaitingForData:
		err = p._WaitingForData(nextMsg)
	case statusWaitingForOk:
		err = p._WaitingForOK(nextMsg)
	case statusWaitingForReject:
		err = p._WaitingForReject(nextMsg)
	}

	return
}

func (p *authState) _WaitingForData(msg []string) error {
	switch msg[0] {
	case "DATA":
		return ErrAuthUnknownCommand
	case "REJECTED":
		return ErrAuthFailed
	case "OK":
		p._Send("BEGIN")
		p.status = statusAuthenticated
	default:
		p._Send("ERROR")
		p.status = statusWaitingForData
	}
	return nil
}

func (p *authState) _WaitingForOK(msg []string) error {
	switch msg[0] {
	case "OK":
		p._Send("BEGIN")
		p.status = statusAuthenticated
	case "REJECT":
		return ErrAuthFailed
	case "DATA", "ERROR":
		p._Send("CANCEL")
		p.status = statusWaitingForReject
	default:
		p._Send("ERROR")
		p.status = statusWaitingForOk
	}

	return nil
}

func (p *authState) _WaitingForReject(msg []string) error {
	switch msg[0] {
	case "REJECT":
		return ErrAuthFailed
	default:
		return ErrAuthUnknownCommand
	}
	return nil
}
