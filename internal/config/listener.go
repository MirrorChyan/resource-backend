package config

type KeyListener struct {
	Key      string
	Listener func(any)
}

var listeners []KeyListener

// RegisterKeyListener use in init method don't dynamic update
func RegisterKeyListener(l KeyListener) {
	listeners = append(listeners, l)
}
