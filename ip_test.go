package rio_test

import (
	"github.com/brickingsoft/rio"
	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"net"
	"sync"
	"testing"
)

func TestIP(t *testing.T) {
	//raddr := &net.IPAddr{
	//	IP:   net.ParseIP("127.0.0.1"),
	//	Zone: "",
	//}
	//srv, srvErr := net.ListenIP("ip", raddr)
	srv, srvErr := rio.ListenPacket("ip4:icmp", "127.0.0.1")
	if srvErr != nil {
		t.Error(srvErr)
		return
	}
	wg := new(sync.WaitGroup)

	wg.Add(1)
	go func(conn net.PacketConn, wg *sync.WaitGroup) {
		defer wg.Done()
		t.Log("srv:", conn.LocalAddr())
		b := make([]byte, 1024)
		rn, addr, rErr := srv.ReadFrom(b)
		t.Log("srv read", rn, addr, rErr)
		if rErr != nil {
			_ = conn.Close()
			return
		}
		msg, _ := icmp.ParseMessage(ipv4.ICMPTypeEchoReply.Protocol(), b[0:rn])
		bb, _ := msg.Marshal(nil)
		wn, wErr := conn.WriteTo(bb, addr)
		t.Log("srv write", wn, wErr)
		_ = conn.Close()
		t.Log("srv done")
		return
	}(srv, wg)

	cli, cliErr := rio.Dial("ip4:icmp", "127.0.0.1")
	if cliErr != nil {
		t.Error(cliErr)
		return
	}
	t.Log("cli:", cli.LocalAddr(), cli.RemoteAddr())

	msg := make([]byte, 8)
	msg[0] = 8  // ICMP echo
	msg[1] = 0  // Code 0
	msg[2] = 0  // Checksum, to be filled later
	msg[3] = 0  // Checksum, to be filled later
	msg[4] = 0  // Identifier
	msg[5] = 13 // Identifier
	msg[6] = 0  // Sequence number
	msg[7] = 37 // Sequence number
	// Calculate checksum
	check := checkSum(msg)
	msg[2] = byte(check >> 8)
	msg[3] = byte(check & 255)
	wn, wErr := cli.Write(msg)
	if wErr != nil {
		cli.Close()
		t.Error(wErr)
		return
	}
	t.Log("cli write:", wn)

	b := make([]byte, 20)
	rn, rErr := cli.Read(b)
	t.Log("cli read", rn, rErr)
	if rErr != nil {
		cli.Close()
		t.Error(rErr)
		return
	}
	cli.Close()
	t.Log("cli done")
	wg.Wait()
	t.Log("fin")
}

func checkSum(data []byte) uint16 {
	var sum uint32
	for i := 0; i < len(data)-1; i += 2 {
		sum += uint32(data[i])<<8 + uint32(data[i+1])
	}
	return ^uint16(sum + sum>>16)
}

// 定义 ICMP 报文
type ICMP struct {
	Type        uint8  // 类型
	Code        uint8  // 代码
	Checksum    uint16 // 校验和
	Identifier  uint16 // 标识符
	SequenceNum uint16 // 序列号
}

func CheckSum(data []byte) (rt uint16) {
	var (
		sum    uint32
		length int = len(data)
		index  int
	)
	for length > 1 {
		sum += uint32(data[index])<<8 + uint32(data[index+1])
		index += 2
		length -= 2
	}
	if length > 0 {
		sum += uint32(data[index]) << 8
	}
	rt = uint16(sum) + uint16(sum>>16)
	return ^rt
}
