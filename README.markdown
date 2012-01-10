Documentation
=============

Look at the API on [GoPkgDoc](http://gopkgdoc.appspot.com/pkg/github.com/norisatir/go-dbus).

Installation
============

    goinstall github.com/norisatir/go-dbus

Usage
=====

```go
import "github.com/norisatir/go-dbus"
import "log"

func main() {
    var (
        err error
        conn *dbus.Connection
        out []interface{}
    )

    // Connect to Session or System buses.
    if conn, err = dbus.Connect(dbus.SessionBus); err != nil {
        log.Fatal("Connection error:", err)
    }
    if err = conn.Initialize(); err != nil {
        log.Fatal("Initialization error:", err)
    }

    // Get objects.
    obj := conn.GetObject("org.freedesktop.Notifications", "/org/freedesktop/Notifications")

    // Introspect objects.
    out, err = conn.CallMethod(
        conn.Interface(obj, "org.freedesktop.DBus.Introspectable"),
        "Instrospect")
    if err != nil {
        log.Fatal("Introspect error:", err)
    }
    var intro dbus.Introspect
    intro, err := dbus.NewIntrospect(out[0])
    method := intro.GetInterfaceData("org.freedesktop.Notifications").GetMethodData("Notify")
    log.Printf("%s(%s) (%s)", method.GetName(), method.GetInSignature(), method.GetOutSigniture())

    // Call object methods.
    out, err = conn.CallMethod(
        conn.Interface(
            conn.GetObject("org.freedesktop.Notifications", "/org/freedesktop/Notifications"),
            "org.freedesktop.Notifications"),
        "Notify",
		"dbus-tutorial", uint32(0), "",
        "dbus-tutorial", "You've been notified!",
		[]interface{}{}, map[string]interface{}{}, int32(-1))
    )
    if err != nil {
        log.Fatal("Notification error:", err)
    }
    log.Print("Notification id:", out[0])
}
```
