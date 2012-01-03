package dbus

import "github.com/kr/pretty.go"

var typeMap = map[MessageType]string{
	INVALID:       "invalid",
	METHOD_CALL:   "method_call",
	METHOD_RETURN: "method_return",
	SIGNAL:        "signal",
	ERROR:         "error",
}

type MatchRule struct {
	Type      string
	Interface string
	Member    string
	Path      string
}

func (p *MatchRule) _ToString() string {
	return pretty.Sprintf("%# v", p)
}

func (p *MatchRule) _Match(msg *Message) bool {
	if p.Type != "" && p.Type != typeMap[msg.Type] {
		return false
	}
	if p.Interface != "" && p.Interface != msg.Iface {
		return false
	}
	if p.Member != "" && p.Member != msg.Member {
		return false
	}
	if p.Path != "" && p.Path != msg.Path {
		return false
	}
	return true
}
