package dbus

import (
	"testing"
	"fmt"
)

func TestDBus(t *testing.T){
	con,err := NewSessionBus()
	if err != nil { t.Fatal(err.Error())}

	err = con.Initialize()

	if err != nil { t.Error("#1 Failed")}

	obj := con.GetObject("org.freedesktop.Notifications", "/org/freedesktop/Notifications")

	inf := con.Interface(obj,"org.freedesktop.Notifications")
	if inf == nil { t.Error("Failed #3")}

	ret,_ := con.CallMethod(inf, "Notify", "dbus.go", uint32(0), "info", "test", "test_body", []string{}, map[uint32] interface{}{}, int32(2000))
	fmt.Println(ret)

	
}
