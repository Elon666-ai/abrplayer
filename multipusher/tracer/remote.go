package tracer

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"net"
	"strings"
	"sync"
	"time"
	"unsafe"
)

const log_indent = 16
const max_remote_queue_size = 10000
const max_local_queue_size = 65535
const max_dataLen  =  1000	//remote
const max_dataLen_local  =  5000

type PK_LOG_HEADER struct {
	cmd     uint16
	size    uint16
	session uint16
	seq     uint16
}

type PK_LOG_MSG struct {
	Ident    [log_indent]byte
	Time     uint64
	ID uint8
	Level    uint8
	//Data     [1]byte
}

type logmsg struct {
	logtime  uint64
	ID uint8
	level    uint8
	data     string
}

type remote struct {
	ident         string
	addr          string
	conn          net.Conn
	mutex         sync.Mutex
	msgQueue      []*logmsg
	msgQueueMutex sync.Mutex
	isConnected   bool
	seqnum        uint16
	stopCh        chan struct{}
	stopOnce      sync.Once
}

func newRemote() *remote {
	return &remote{
		msgQueue:    make([]*logmsg, 0),
		isConnected: false,
		seqnum:      0,
		stopCh:      make(chan struct{}),
	}
}

func (r *remote) start(ident string, addr string) {
	r.ident = ident
	r.addr = addr

	AsyncRunCoroutine(func() {
		r.checkstatus()
	})

	AsyncRunCoroutine(func() {
		r.processqueue()
	})
}

// Close terminates the background goroutines started by start. It is safe to
// call multiple times.
func (r *remote) Close() {
	r.stopOnce.Do(func() {
		close(r.stopCh)
	})
	r.mutex.Lock()
	if r.conn != nil {
		_ = r.conn.Close()
	}
	r.mutex.Unlock()
}

func (r *remote) pop_front() *logmsg {
	r.msgQueueMutex.Lock()
	defer r.msgQueueMutex.Unlock()
	if r.isConnected && len(r.msgQueue) > 0 {
		d := r.msgQueue[0]
		r.msgQueue = r.msgQueue[1:]
		return d
	}
	return nil
}

func (r *remote) push_front(msg *logmsg) {
	r.msgQueueMutex.Lock()
	defer r.msgQueueMutex.Unlock()
	r.msgQueue = append([]*logmsg{msg}, r.msgQueue...)
}

func (r *remote) processqueue() {
	for {
		select {
		case <-r.stopCh:
			return
		default:
		}
		d := r.pop_front()
		if d != nil {
			err := r.sendlog(d)
			if err != nil {
				r.push_front(d)
			}
		} else {
			select {
			case <-r.stopCh:
				return
			case <-time.After(20 * time.Millisecond):
			}
		}
	}
}

func (r *remote) nextseq() uint16  {
	r.seqnum++
	return r.seqnum
}

func (r* remote) sendlog(info *logmsg) error{
	msg := PK_LOG_MSG{}
	copy(msg.Ident[:], r.ident)
	msg.ID = info.ID
	msg.Level = info.level
	msg.Time = info.logtime

	bodyBuf := new(bytes.Buffer)
	_ = binary.Write(bodyBuf, binary.BigEndian, msg)
	header := PK_LOG_HEADER{}
	header.cmd = 0x0001
	header.seq = r.nextseq()
	header.size = uint16(unsafe.Sizeof(header)) + uint16(len(bodyBuf.Bytes())) + uint16(len(info.data))

	headerBuf := new(bytes.Buffer)
	_ = binary.Write(headerBuf, binary.BigEndian, header)
	data := make([]byte, 0)
	data = append(data, headerBuf.Bytes()...)
	data = append(data, bodyBuf.Bytes()...)
	data = append(data, []byte(info.data)...)
	return r.sendData(data)
}

func (r *remote) keepAlive() {
	header := PK_LOG_HEADER{}
	header.cmd = 0x0000
	header.seq = r.nextseq()
	header.size = uint16(unsafe.Sizeof(header))
	headerBuf := new(bytes.Buffer)
	_ = binary.Write(headerBuf, binary.BigEndian, header)
	r.sendData(headerBuf.Bytes())
}

func (r *remote) sendData(buf []byte) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	if r.conn == nil {
		return errors.New("remote log connection is nil")
	}
	totalLen := len(buf)
	for totalLen > 0 {
		r.conn.SetWriteDeadline(time.Now().Add(time.Second * 60))
		n, err := r.conn.Write(buf)
		if err != nil {
			r.closeLocked()
			LogNotice(ID_APP, "logserver %v disconnected, err=%v", r.addr, err)
			return err
		}
		totalLen -= n
		buf = buf[n:]
	}
	return nil
}

func (r *remote) checkstatus() {
	timer := time.NewTicker(time.Second)
	defer timer.Stop()
	internalSecs := 1
	for {
		select {
		case <-r.stopCh:
			return
		case _, ok := <-timer.C:
			if !ok {
				return
			}
			if r.isConnected {
				if internalSecs >= 5 {
					internalSecs = 1
					r.keepAlive()
				} else {
					internalSecs++
				}
			} else {
				r.connect(r.addr)
			}
		}
	}
}

func (r *remote) close() {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.closeLocked()
}

// closeLocked closes the connection assuming r.mutex is already held.
func (r *remote) closeLocked() {
	r.isConnected = false
	if r.conn != nil {
		_ = r.conn.Close()
	}
}

func (r *remote) connect(addr string) error {
	LogInfo(ID_APP, "try to connect logserver %v", addr)
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		LogNotice(ID_APP, "connect logserver %v failed, err=%v", addr, err)
		return nil
	}
	r.mutex.Lock()
	r.conn = conn
	r.isConnected = true
	r.mutex.Unlock()
	LogInfo(ID_APP, "logserver %v connected success", addr)

	// Capture conn locally so the read loop is not racing on r.conn after
	// reconnect/close swaps it out.
	fn := func() {
		buf := make([]byte, 1024)
		for {
			_, err := conn.Read(buf)
			if err != nil {
				r.close()
				LogNotice(ID_APP, "logserver %v disconnected, err=%v", addr, err)
				return
			}
		}
	}
	AsyncRunCoroutine(fn)
	return nil
}

func (r *remote) put(millsecs uint64, ID uint8, level uint8, data string) {
	if 0 == len(data){
		return
	}

	str := data
	nLen := len(data)
	if nLen > max_dataLen{
		str = data[:max_dataLen]
	}
	// replace \n
	str = strings.Replace(str, "\n", " ", math.MaxInt8)

	d := &logmsg{
		logtime:  millsecs,
		ID: ID,
		level:    level,
		data:     str,
	}

	r.msgQueueMutex.Lock()
	r.msgQueue = append(r.msgQueue, d)
	if len(r.msgQueue) > max_remote_queue_size {
		r.msgQueue = r.msgQueue[1:]
	}
	r.msgQueueMutex.Unlock()
}
