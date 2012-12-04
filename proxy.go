package dbus

// This is not yet finished: it is an idea for what statically generated object bindings could look like.

type Introspectable struct {
	*ObjectProxy
}

func (o *Introspectable) Introspect() (data string, err error) {
	reply, err := o.Call("org.freedesktop.DBus.Introspectable", "Introspect")
	if err != nil {
		return
	}
	err = reply.GetArgs(&data)
	return
}

type Properties struct {
	*ObjectProxy
}

func (o *Properties) Get(interfaceName string, propertyName string) (value interface{}, err error) {
	reply, err := o.Call("org.freedesktop.DBus.Properties", "Get", interfaceName, propertyName)
	if err != nil {
		return
	}
	var variant Variant
	err = reply.GetArgs(&variant)
	value = variant.Value
	return
}

func (o *Properties) Set(interfaceName string, propertyName string, value interface{}) (err error) {
	_, err = o.Call("org.freedesktop.DBus.Properties", "Set", interfaceName, propertyName, Variant{value})
	return
}

func (o *Properties) GetAll(interfaceName string) (props map[string]Variant, err error) {
	reply, err := o.Call("org.freedesktop.DBus.Properties", "GetAll", interfaceName)
	if err != nil {
		return
	}
	err = reply.GetArgs(&props)
	return
}

type MessageBus struct {
	*ObjectProxy
}

func (o *MessageBus) Hello() (uniqueName string, err error) {
	reply, err := o.Call("org.freedesktop.DBus", "Hello")
	if err != nil {
		return
	}
	err = reply.GetArgs(&uniqueName)
	return
}

func (o *MessageBus) RequestName(name string, flags uint32) (result uint32, err error) {
	reply, err := o.Call("org.freedesktop.DBus", "RequestName", name, flags)
	if err != nil {
		return
	}
	err = reply.GetArgs(&result)
	return
}

func (o *MessageBus) ReleaseName(name string) (result uint32, err error) {
	reply, err := o.Call("org.freedesktop.DBus", "ReleaseName", name)
	if err != nil {
		return
	}
	err = reply.GetArgs(&result)
	return
}

func (o *MessageBus) ListQueuedOwners(name string) (owners []string, err error) {
	reply, err := o.Call("org.freedesktop.DBus", "ListQueuedOwners", name)
	if err != nil {
		return
	}
	err = reply.GetArgs(&owners)
	return
}

func (o *MessageBus) ListNames() (names []string, err error) {
	reply, err := o.Call("org.freedesktop.DBus", "ListNames")
	if err != nil {
		return
	}
	err = reply.GetArgs(&names)
	return
}

func (o *MessageBus) ListActivatableNames() (names []string, err error) {
	reply, err := o.Call("org.freedesktop.DBus", "ListActivatableNames")
	if err != nil {
		return
	}
	err = reply.GetArgs(&names)
	return
}

func (o *MessageBus) NameHasOwner(name string) (hasOwner bool, err error) {
	reply, err := o.Call("org.freedesktop.DBus", "NameHasOwner", name)
	if err != nil {
		return
	}
	err = reply.GetArgs(&hasOwner)
	return
}

func (o *MessageBus) StartServiceByName(name string, flags uint32) (result uint32, err error) {
	reply, err := o.Call("org.freedesktop.DBus", "StartServiceByName", name, flags)
	if err != nil {
		return
	}
	err = reply.GetArgs(&result)
	return
}

func (o *MessageBus) UpdateActivationEnvironment(env map[string]string) (err error) {
	_, err = o.Call("org.freedesktop.DBus", "UpdateActivationEnvironment", env)
	return
}

func (o *MessageBus) GetNameOwner(name string) (owner string, err error) {
	reply, err := o.Call("org.freedesktop.DBus", "GetNameOwner", name)
	if err != nil {
		return
	}
	err = reply.GetArgs(&owner)
	return
}

func (o *MessageBus) GetConnectionUnixUser(busName string) (user uint32, err error) {
	reply, err := o.Call("org.freedesktop.DBus", "GetConnectionUnixUser", busName)
	if err != nil {
		return
	}
	err = reply.GetArgs(&user)
	return
}

func (o *MessageBus) GetConnectionUnixProcessID(busName string) (process uint32, err error) {
	reply, err := o.Call("org.freedesktop.DBus", "GetConnectionUnixProcessID", busName)
	if err != nil {
		return
	}
	err = reply.GetArgs(&process)
	return
}

func (o *MessageBus) AddMatch(rule string) (err error) {
	_, err = o.Call("org.freedesktop.DBus", "AddMatch", rule)
	return
}

func (o *MessageBus) RemoveMatch(rule string) (err error) {
	_, err = o.Call("org.freedesktop.DBus", "RemoveMatch", rule)
	return
}

func (o *MessageBus) GetId() (busId string, err error) {
	reply, err := o.Call("org.freedesktop.DBus", "GetId")
	if err != nil {
		return
	}
	err = reply.GetArgs(&busId)
	return
}
