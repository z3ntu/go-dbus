
package dbus

import(
	"testing"
)

func TestToString(t *testing.T){
	verifyStr := "&dbus.MatchRule{\n" +
"	Type:      \"signal\",\n" +
"	Interface: \"org.freedesktop.DBus\",\n"+
"	Member:    \"Foo\",\n"+
"	Path:      \"/bar/foo\",\n"+
"}"

	mr := MatchRule{
	  Type:"signal",
	  Interface:"org.freedesktop.DBus",
  	Member:"Foo",
	  Path:"/bar/foo"}

	if mr._ToString() != verifyStr { t.Error("#1 Failed")}
}
