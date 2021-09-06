package minecraft

import (
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

const maxResponseSize = 4110 // https://wiki.vg/Rcon#Fragmentation

var (
	errAuthenticationFailure = errors.New("failed to authenticate")
	errInvalidResponseID     = errors.New("invalid response ID")
)

// Client manages a connection to a Minecraft server.
type Client struct {
	conn   net.Conn
	lastID int32
	lock   sync.Mutex

	timeout time.Duration
}

// NewClient creates a TCP connection to a Minecraft server.
func NewClient(hostport string) (*Client, error) {
	conn, err := net.DialTimeout("tcp", hostport, 1*time.Second)
	if err != nil {
		return nil, err
	}
	return &Client{conn: conn}, nil
}

func NewClientTimeout(hostport string, timeout time.Duration) (*Client, error) {
	conn, err := net.DialTimeout("tcp", hostport, timeout)
	if err != nil {
		return nil, err
	}
	return &Client{conn: conn, timeout: timeout}, nil
}

// Close disconnects from the server.
func (c *Client) Close() error {
	return c.conn.Close()
}

// Authenticate starts a logged-in RCON session.
func (c *Client) Authenticate(password string) error {
	if _, err := c.sendMessage(MsgAuthenticate, password); err != nil {
		// When invalid credentials are supplied, the server will return a non-matching response ID.
		if err == errInvalidResponseID {
			return errAuthenticationFailure
		}
		return err
	}
	return nil
}

// SendCommand sends an RCON command to the server.
func (c *Client) SendCommand(command string) (Message, error) {
	return c.sendMessage(MsgCommand, command)
}

// sendMessage uses the client's underlying TCP connection to send and receive data.
func (c *Client) sendMessage(msgType MessageType, msg string) (Message, error) {
	request := Message{
		Length: int32(len(msg) + headerSize),
		ID:     atomic.AddInt32(&c.lastID, 1),
		Type:   msgType,
		Body:   msg,
	}

	encoded, err := EncodeMessage(request)
	if err != nil {
		return Message{}, err
	}

	respBytes, err := c.sendEncodedMessage(encoded)
	if err != nil {
		return Message{}, err
	}

	resp, err := DecodeMessage(respBytes)
	if err != nil {
		return Message{}, err
	}

	if resp.ID != request.ID {
		return Message{}, errInvalidResponseID
	}

	return resp, nil
}

func (c *Client) sendEncodedMessage(encoded []byte) ([]byte, error) {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.conn.SetWriteDeadline(time.Now().Add(c.timeout))
	if _, err := c.conn.Write(encoded); err != nil {
		return nil, err
	}

	respBytes := make([]byte, maxResponseSize)
	c.conn.SetReadDeadline(time.Now().Add(c.timeout))
	read, err := c.conn.Read(respBytes)
	if err != nil {
		return nil, err
	}

	return respBytes[:read], nil
}
