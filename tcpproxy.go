package main

import (
	"bytes"
	"compress/flate"
	// "encoding/binary"
	"compress/zlib"
	//"encoding/hex"
	"flag"
	"fmt"
	"github.com/axgle/mahonia"
	"io"
	"io/ioutil"
	"net"
	"os"
	"os/signal"
	"reflect"
	"strconv"
	"syscall"
	"unsafe"
)

/**
 *  tcp 启动 链接 三次握手  关闭四次
 *  慢启动 + 拥塞窗口
 *  门限控制 < 拥塞窗口 进入拥塞避免
 *  在发送数据后 检测 ack 确认超时 或者 收到重复ack 确认 操作门限值 和 拥塞窗口值 来限流
 *  接收方 使用通告窗口 告知发送可接受多少字节
 *
 *  发送方：滑动窗口协议
 */
var MaxPayloadLen int = 1<<24 - 1

//结构体内嵌接口
type Man struct {
	iTest
	name string
}

type iTest interface {
	hello()
}

type Em struct {
	good string
}

func (em *Em) hello() {
	fmt.Println(em.good)
}

func (man *Man) hello() {
	fmt.Println(man.name)
}

var workerChan = make(chan *net.TCPConn, 10000)
var graddr *string

func main() {

	// vman := Man{
	// 	&Em{
	// 		good: "goodman",
	// 	},
	// 	"helloman"}
	// vman.hello()
	// fmt.Println(vman)
	//gb := make([]byte, 15000000)
	nf := flag.String("name", "server", "input server or client")
	graddr = flag.String("rip", "192.168.1.19:3306", "ip:port")
	lsaddr := flag.String("lip", "0.0.0.0:9696", "local ip")
	flag.Parse()

	fmt.Println(*nf)
	fmt.Println("本地监听：", *lsaddr)
	fmt.Println("本程序默认链接是192.168.1.19：3306 的 MYSQL")
	fmt.Println("请将程序的MYSQL链接地址指向到本机：", *lsaddr)
	go signalHandler()

	laddr, aerr := net.ResolveTCPAddr("tcp", *lsaddr)
	if aerr != nil {
		os.Exit(1)
	}
	listen, lerr := net.ListenTCP("tcp", laddr)
	defer listen.Close()
	if lerr != nil {
		os.Exit(2)
	}

	for {
		tcpConn, cerr := listen.AcceptTCP()
		fmt.Println("accept one client")
		if cerr == nil {
			go clientRequest(tcpConn) //plan 1
		}
	}

	var wait string
	fmt.Scanln(&wait)
}

func signalHandler() {
	ch := make(chan os.Signal)
	signal.Notify(ch, syscall.SIGTERM)
	for {
		switch <-ch {
		case syscall.SIGTERM:
			//ftpServer.Stop()
			os.Exit(100)
			break
		}
	}
}

var debug bool = false

/**
 * 工作协程
 */
func clientRequest(conn *net.TCPConn) {
	// 	//接受偏移
	// var recvpos int

	// //解析偏移
	// var pos int

	// //写入偏移
	// var writepos int
	var recvpos = 0
	var pos = 0
	var writepos = 0
	var num = 0
	raddr, rerr := net.ResolveTCPAddr("tcp", *graddr)
	if rerr != nil {
		fmt.Println(rerr)
		return
		//os.Exit(3)
	}
	mysqltcpConn, dterr := net.DialTCP("tcp", nil, raddr)

	if dterr != nil {
		fmt.Println(dterr)
		return
	}
	go mysqlReturn(conn, mysqltcpConn)

	var b [15000000]byte
	var com = false
	for {
		num++
		rlen, err := conn.Read(b[recvpos:])
		if debug {

			fmt.Println("recv length:", rlen)
		}
		if err != nil {
			fmt.Println("mysql client退出")
			fmt.Println(err)
			return
		} else {
			//增加接受偏移
			recvpos += rlen
		}

		packContentlength := int(uint32(b[pos+0]) | uint32(b[pos+1])<<8 | uint32(b[pos+2])<<16)
		// if packContentlength < 1 {
		// 	fmt.Println("不可用的头部")
		// }

		packnum := uint8(b[pos+3])
		pos += 4

		packcommd := uint8(b[pos+4])
		if debug {
			fmt.Println("packContentLength:", packContentlength, "packnum:", packnum)
		}
		if packnum == 0 && packcommd == 0 {
			pos += 3
			packContentlength = int(uint32(b[pos+0]) | uint32(b[pos+1])<<8 | uint32(b[pos+2])<<16)
			packnum = uint8(b[pos+3])
			packcommd = uint8(b[pos+4])
			pos += 4
			if debug {
				fmt.Println("packContentLength:", packContentlength, "packnum:", packnum, "command", packcommd)
			}
		} else {

			if rlen-7 == packContentlength && uint8(b[pos+3+1]) == 156 && uint8(b[pos+3]) == 120 {
				com = true
				if debug {
					fmt.Println(uint8(b[pos+3]))
					fmt.Println(uint8(b[pos+3+1]))
					fmt.Println("find need compress")
				}
				dedata := DoZlibUnCompress(b[pos+3 : pos+rlen-3])
				fmt.Println("------------")
				fmt.Println("[", num, "]", string(dedata[1:]))

			}
		}
		//readpos
		//-----------recvpos
		//----writepost
		//
		//
		//pos来增加
		// if 4+packContentlength <= rlen {

		// }

		if packContentlength > 0 && pos+packContentlength <= recvpos {

			if com == false {
				fmt.Println("------------")
				fmt.Println("[", num, "]", string(b[pos+1:pos+packContentlength]))

			}
			//+1 去掉 命令位

			pos += packContentlength
		} else {
			pos -= 4
		}
		if err != nil {
			fmt.Println(err)
		}

		mysqltcpConn.Write(b[writepos:recvpos])
		writepos = recvpos
		pos = recvpos
		com = false
		//fmt.Println(hex.EncodeToString(b[0:recvpos]))
	}

}
func btox(b string) string {
	base, _ := strconv.ParseInt(b, 2, 10)
	return strconv.FormatInt(base, 16)
}

const (
	COM_SLEEP byte = iota
	COM_QUIT
	COM_INIT_DB
	COM_QUERY
	COM_FIELD_LIST
	COM_CREATE_DB
	COM_DROP_DB
	COM_REFRESH
	COM_SHUTDOWN
	COM_STATISTICS
	COM_PROCESS_INFO
	COM_CONNECT
	COM_PROCESS_KILL
	COM_DEBUG
	COM_PING
	COM_TIME
	COM_DELAYED_INSERT
	COM_CHANGE_USER
	COM_BINLOG_DUMP
	COM_TABLE_DUMP
	COM_CONNECT_OUT
	COM_REGISTER_SLAVE
	COM_STMT_PREPARE
	COM_STMT_EXECUTE
	COM_STMT_SEND_LONG_DATA
	COM_STMT_CLOSE
	COM_STMT_RESET
	COM_SET_OPTION
	COM_STMT_FETCH
	COM_DAEMON
	COM_BINLOG_DUMP_GTID
	COM_RESET_CONNECTION
)

func String(b []byte) (s string) {
	pbytes := (*reflect.SliceHeader)(unsafe.Pointer(&b))
	pstring := (*reflect.StringHeader)(unsafe.Pointer(&s))
	pstring.Data = pbytes.Data
	pstring.Len = pbytes.Len
	return
}

func ConvertToString(src string, srcCode string, tagCode string) string {
	srcCoder := mahonia.NewDecoder(srcCode)
	srcResult := srcCoder.ConvertString(src)
	tagCoder := mahonia.NewDecoder(tagCode)
	_, cdata, _ := tagCoder.Translate([]byte(srcResult), true)
	result := string(cdata)
	return result
}

func mysqlReturn(conn *net.TCPConn, mysqlconn *net.TCPConn) {
	var b [1500]byte
	for {
		len, err := mysqlconn.Read(b[0:])

		if err != nil {
			//fmt.Println(err)
		}
		conn.Write(b[0:len])
		//conn.Write([]byte("HELLO WORLD"))
	}
}

func FlateDecode(input []byte) (result []byte, err error) {
	fmt.Println(input[0:])
	result, err = ioutil.ReadAll(flate.NewReader(bytes.NewReader(input)))
	fmt.Println(result[0:])
	return result, err
}

func DoZlibUnCompress(compressSrc []byte) []byte {
	b := bytes.NewReader(compressSrc)
	var out bytes.Buffer
	r, _ := zlib.NewReader(b)
	io.Copy(&out, r)
	return out.Bytes()
}

// int ZEXPORT uncompress (dest, destLen, source, sourceLen)
//     Bytef *dest;
//     uLongf *destLen;
//     const Bytef *source;
//     uLong sourceLen;
// {
//     z_stream stream;
//     int err;

//     stream.next_in = (Bytef*)source;
//     stream.avail_in = (uInt)sourceLen;
//     /* Check for source > 64K on 16-bit machine: */
//     if ((uLong)stream.avail_in != sourceLen) return Z_BUF_ERROR;

//     stream.next_out = dest;
//     stream.avail_out = (uInt)*destLen;
//     if ((uLong)stream.avail_out != *destLen) return Z_BUF_ERROR;

//     stream.zalloc = (alloc_func)0;
//     stream.zfree = (free_func)0;

//     err = inflateInit(&stream);
//     if (err != Z_OK) return err;

//     err = inflate(&stream, Z_FINISH);
//     if (err != Z_STREAM_END) {
//         inflateEnd(&stream);
//         if (err == Z_NEED_DICT || (err == Z_BUF_ERROR && stream.avail_in == 0))
//             return Z_DATA_ERROR;
//         return err;
//     }
//     *destLen = stream.total_out;

//     err = inflateEnd(&stream);
//     return err;
// }
