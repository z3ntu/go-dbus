package dbus

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"errors"
	"os"
	"strconv"
)

type Authenticator interface {
	Mechanism() []byte
	InitialResponse() []byte
	ProcessData([]byte) ([]byte, error)
}

type AuthExternal struct {
}

func (p *AuthExternal) Mechanism() []byte {
	return []byte("EXTERNAL")
}

func (p *AuthExternal) InitialResponse() []byte {
	uid := []byte(strconv.Itoa(os.Getuid()))
	uidHex := make([]byte, hex.EncodedLen(len(uid)))
	hex.Encode(uidHex, uid)
	return uidHex
}

func (p *AuthExternal) ProcessData([]byte) ([]byte, error) {
	return nil, errors.New("Unexpected Response")
}

func min(l, r int) int {
	if l < r {
		return l
	}
	return r
}

func (p *Connection) _Authenticate(mech Authenticator) error {
	inStream := bufio.NewReader(p.conn)
	msg := bytes.Join([][]byte{[]byte("AUTH"), mech.Mechanism(), mech.InitialResponse()}, []byte(" "))
	_, err := p.conn.Write(append(msg, "\r\n"...))

	for {
		mesg, _, _ := inStream.ReadLine()

		switch {
		case bytes.HasPrefix(mesg, []byte("DATA")):
			var resp []byte
			resp, err = mech.ProcessData(mesg[min(len("DATA "), len(mesg)):])
			if err != nil {
				p.conn.Write([]byte("CANCEL\r\n"))
			}
			p.conn.Write(append(resp, "\r\n"...))

		case bytes.HasPrefix(mesg, []byte("OK")),
			bytes.HasPrefix(mesg, []byte("AGREE_UNIX_FD")):
			p.conn.Write([]byte("BEGIN\r\n"))
			return nil

		case bytes.HasPrefix(mesg, []byte("REJECTED")):
			if err != nil {
				return err
			}
			return errors.New("Rejected: " + string(mesg[min(len("REJECTED "), len(mesg)):]))

		case bytes.HasPrefix(mesg, []byte("ERROR")):
			return errors.New("Error: " + string(mesg[min(len("ERROR "), len(mesg)):]))

		default:
			p.conn.Write([]byte("ERROR\r\n"))
		}
	}
	return nil
}
