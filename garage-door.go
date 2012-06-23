package main

import (
	"bumbleserver.org/client"
	"bumbleserver.org/common/envelope"
	"bumbleserver.org/common/key"
	"bumbleserver.org/common/message"
	"bumbleserver.org/common/peer"
	"crypto/rsa"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var privateKey *rsa.PrivateKey
var isAuthenticated bool = false
var door *GarageDoor = &GarageDoor{}

func main() {
	privateKeyFile := filepath.Clean("key.pri")
	if !filepath.IsAbs(privateKeyFile) {
		privateKeyFile = filepath.Clean(filepath.Join(filepath.Dir(os.Args[0]), privateKeyFile))
	}

	var err error
	privateKey, err = key.PrivateKeyFromPEMFile(privateKeyFile)
	if err != nil {
		fmt.Printf("[PRIVATEKEY ERROR] %s\n", err)
		os.Exit(1)
	}
	var c *client.Client
	clientConfig := &client.Config{
		Name:             "bumble://bumbleserver.net/asset/dustygaragedoor",
		PrivateKey:       privateKey,
		OnConnect:        onConnect,
		OnDisconnect:     onDisconnect,
		OnAuthentication: onAuthentication,
		OnMessage:        onMessage,
	}
	c = client.NewClient(clientConfig)
	go func() {
		for {
			err = c.Connect() // NOTE: client.Connect never returns unless it has an error
			if err != nil {
				if err.Error() == "got disconnected" {
					fmt.Println("XXX")
				} else {
					fmt.Println(err)
					os.Exit(1)
				}
			}
			<-time.NewTimer(time.Duration(5e9)).C // 5 second delay before re-looping
		}
	}()
	door.mover()
}

func onConnect(c *client.Client) {
	fmt.Println("We connected.")
}

func onDisconnect(c *client.Client) {
	fmt.Println("We disconnected or got disconnected.")
}

func onAuthentication(c *client.Client, success bool, error string) {
	fmt.Printf("Did we authenticate?  %t\t%s\n", success, error)
	isAuthenticated = success
}

func onMessage(c *client.Client, e *envelope.Envelope, m *message.Header) {
	fmt.Printf("We got a message: %s\n", m)
	if m.GetType() == 0 && m.GetCode() == 200 {
		msg := e.GetMessage(privateKey)
		gm, err := message.GenericParse(msg)
		if err != nil {
			fmt.Printf("Tried to GenericParse the message but it failed: %s\n", err.Error())
			return
		}
		switch strings.ToUpper(gm.GetInfo()) {
		case "OPEN":
			door.Open()
			door.getState(c, e.GetFrom())
		case "CLOSE":
			door.Close()
			door.getState(c, e.GetFrom())
		case "STOP":
			door.Stop()
			door.getState(c, e.GetFrom())
		case "GETPOSITION":
			door.getPosition(c, e.GetFrom())
		case "GETDIRECTION":
			door.getDirection(c, e.GetFrom())
		case "GETSTATE":
			door.getState(c, e.GetFrom())
		default:
			fmt.Printf("Received an unknown command: %s\n", gm.GetInfo())
		}
	}
}

////////////

func sendMessage(c *client.Client, to *peer.Peer, text string) {
	msg := message.NewGeneric(200)
	msg.SetTo(to)
	msg.SetInfo(text)
	c.OriginateMessage(msg)
}

////////////

type GarageDoor struct {
	State     GarageDoorState
	Direction GarageDoorDirection
	Position  float32 // 0 = closed full, 1 = open full
}

type GarageDoorState int

const (
	GD_CLOSED GarageDoorState = iota
	GD_CLOSING
	GD_STOPPED
	GD_OPENING
	GD_OPEN
)

type GarageDoorDirection bool

const (
	GD_CLOSEDIR GarageDoorDirection = false
	GD_OPENDIR  GarageDoorDirection = true
)

func (door *GarageDoor) Open() {
	fmt.Println("WISH TO OPEN GARAGE DOOR.")
	if door.State == GD_OPEN {
		fmt.Println("Garage door is already open.")
	} else if door.State == GD_OPENING {
		fmt.Println("Garage door is already opening.")
	} else if door.State == GD_STOPPED {
		fmt.Println("Garage door is stopped, but we'll tell it to open now.")
		door.sendSignal()
	} else if door.State == GD_CLOSED {
		fmt.Println("Garage door is closed, but we'll tell it to open now.")
		door.sendSignal()
	} else if door.State == GD_CLOSING {
		fmt.Println("Garage door is closing, but we'll tell it to stop and then open now.")
		door.Stop()
		door.Open()
	}
}

func (door *GarageDoor) Close() {
	fmt.Println("WISH TO CLOSE GARAGE DOOR.")
	if door.State == GD_OPEN {
		fmt.Println("Garage door is open, but we'll tell it to close now.")
		door.sendSignal()
	} else if door.State == GD_OPENING {
		fmt.Println("Garage door is opening, but we'll tell it to stop and then close.")
		door.Stop()
		door.Close()
	} else if door.State == GD_STOPPED {
		fmt.Println("Garage door is stopped, but we'll tell it to close now.")
		door.sendSignal()
		door.Close()
	} else if door.State == GD_CLOSED {
		fmt.Println("Garage door is already closed.")
	} else if door.State == GD_CLOSING {
		fmt.Println("Garage door is already closing.")
	}
}

func (door *GarageDoor) Stop() {
	fmt.Println("WISH TO STOP GARAGE DOOR.")
	if door.State == GD_OPEN {
		fmt.Println("Garage door is open and therefore not moving.")
	} else if door.State == GD_OPENING {
		fmt.Println("Garage door is opening, but we'll tell it to stop moving.")
		door.sendSignal()
	} else if door.State == GD_STOPPED {
		fmt.Println("Garage door is already stopped.")
	} else if door.State == GD_CLOSED {
		fmt.Println("Garage door is closed and therefore not moving.")
	} else if door.State == GD_CLOSING {
		fmt.Println("Garage door is closing, but we'll tell it to stop moving.")
		door.sendSignal()
	}
}

func (door *GarageDoor) sendSignal() {
	fmt.Println("SENDING TOGGLE SIGNAL TO GARAGE DOOR")
	if door.State == GD_OPEN {
		fmt.Println("Garage door was open but is now closing.")
		door.State = GD_CLOSING
		door.Direction = GD_CLOSEDIR
	} else if door.State == GD_OPENING {
		fmt.Println("Garage door is opening, but we'll tell it to stop moving.")
		door.State = GD_STOPPED
	} else if door.State == GD_STOPPED {
		if door.Direction { // true = open direction
			fmt.Println("Garage door is stopped and the last direction was OPEN, so now we're closing.")
			door.State = GD_CLOSING
			door.Direction = GD_CLOSEDIR
		} else {
			fmt.Println("Garage door is stopped and the last direction was CLOSE, so now we're opening.")
			door.State = GD_OPENING
			door.Direction = GD_OPENDIR
		}
	} else if door.State == GD_CLOSED {
		fmt.Println("Garage door was closed but is now opening.")
		door.State = GD_OPENING
		door.Direction = GD_OPENDIR
	} else if door.State == GD_CLOSING {
		fmt.Println("Garage door is closing, but we'll tell it to stop moving.")
		door.State = GD_STOPPED
	}
}

func (door *GarageDoor) getPosition(c *client.Client, p *peer.Peer) {
	fmt.Println("WISH TO GET POSITION FOR GARAGE DOOR.")
	fmt.Printf("Garage door position is %.02f\n", door.Position)
	sendMessage(c, p, fmt.Sprintf("Garage door position is %.02f\n", door.Position))
}

func (door *GarageDoor) getState(c *client.Client, p *peer.Peer) {
	fmt.Println("WISH TO GET STATE FOR GARAGE DOOR.")
	if door.State == GD_OPEN {
		fmt.Println("Garage door is open.")
		sendMessage(c, p, "Garage door is open.")
	} else if door.State == GD_OPENING {
		fmt.Println("Garage door is opening.")
		sendMessage(c, p, "Garage door is opening.")
	} else if door.State == GD_STOPPED {
		fmt.Println("Garage door is stopped.")
		sendMessage(c, p, "Garage door is stopped.")
	} else if door.State == GD_CLOSED {
		fmt.Println("Garage door is closed.")
		sendMessage(c, p, "Garage door is closed.")
	} else if door.State == GD_CLOSING {
		fmt.Println("Garage door is closing.")
		sendMessage(c, p, "Garage door is closing.")
	}
}

func (door *GarageDoor) getDirection(c *client.Client, p *peer.Peer) {
	fmt.Println("WISH TO GET DIRECTION FOR GARAGE DOOR.")
	if door.Direction { // true = open direction
		fmt.Println("The direction of the door is OPEN direction.")
		sendMessage(c, p, "The direction of the door is OPEN direction.")
	} else {
		fmt.Println("The direction of the door is CLOSE direction.")
		sendMessage(c, p, "The direction of the door is CLOSE direction.")
	}
}

func (door *GarageDoor) mover() {
	// this function might be what monitors the physical state of the garage door and updates the door.Position, door.State, and door.Direction values (in case of manual door operation, for example) 
	for {
		if door.State == GD_CLOSING {
			door.Position -= 0.01
			if door.Position < 0 {
				door.Position = 0
				door.State = GD_CLOSED
				fmt.Println("Garage door has finished closing and is now closed.")
			}
			fmt.Printf("Garage door position is %.02f\n", door.Position)
		} else if door.State == GD_OPENING {
			door.Position += 0.01
			if door.Position > 1 {
				door.Position = 1
				door.State = GD_OPEN
				fmt.Println("Garage door has finished opening and is now open.")
			}
			fmt.Printf("Garage door position is %.02f\n", door.Position)
		}
		<-time.NewTimer(time.Duration(3e8)).C // 0.3 second delay before re-looping
	}
}
