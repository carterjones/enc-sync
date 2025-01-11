package main

type DisconnectError struct{}

func (e DisconnectError) Error() string {
	return "error: disconnected"
}
