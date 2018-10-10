package main

import (
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/netsec-ethz/rains/internal/pkg/section"

	log "github.com/inconshreveable/log15"

	"github.com/netsec-ethz/rains/internal/pkg/cbor"
	"github.com/netsec-ethz/rains/internal/pkg/connection"
	"github.com/netsec-ethz/rains/internal/pkg/message"
	"github.com/netsec-ethz/rains/internal/pkg/object"
	"github.com/netsec-ethz/rains/internal/pkg/query"
	"github.com/netsec-ethz/rains/internal/pkg/token"
	"github.com/netsec-ethz/rains/internal/pkg/util"
	"github.com/netsec-ethz/rains/internal/pkg/zonefile"
)

var anyQuery = []object.Type{object.OTName, object.OTIP4Addr,
	object.OTIP6Addr, object.OTDelegation, object.OTServiceInfo, object.OTRedirection}

//TODO add default values to description
var revLookup = flag.String("x", "", "Reverse lookup, addr is an IPv4 address in dotted-decimal notation, or a colon-delimited IPv6 address.")
var queryType = flag.Int("t", -1, "specifies the type for which dig issues a query.")
var name = flag.String("q", "", "sets the query's subjectName to this value.")
var port = flag.Uint("p", 5022, "is the port number that dig will send its queries to.")
var serverAddr = flag.String("s", "", `is the IP address of the name server to query.
		This can be an IPv4 address in dotted-decimal notation or an IPv6 address in colon-delimited notation.`)
var context = flag.String("c", ".", "context specifies the context for which dig issues a query.")
var expires = flag.Int64("exp", time.Now().Add(10*time.Second).Unix(), "expires sets the valid until value of the query.")
var filePath = flag.String("filePath", "", "specifies a file path where the query's response is appended to")
var insecureTLS = flag.Bool("insecureTLS", false, "when set it does not check the validity of the server's TLS certificate.")
var queryOptions qoptFlag

var zfParser zonefile.ZoneFileParser

func init() {
	zfParser = zonefile.Parser{}
	//TODO CFE this list should be generated from internal constants
	flag.Var(&queryOptions, "qopt", `specifies which query options are added to the query. Several query options are allowed. The sequence in which they are given determines the priority in descending order. Supported values are:
	1: Minimize end-to-end latency
	2: Minimize last-hop answer size (bandwidth)
	3: Minimize information leakage beyond first hop
	4: No information leakage beyond first hop: cached answers only
	5: Expired assertions are acceptable
	6: Enable query token tracing
	7: Disable verification delegation (client protocol only)
	8: Suppress proactive caching of future assertions
	e.g. to specify query options 4 and 2 with higher priority on option 4 write: -qopt=4 -qopt=2
	`)
}

//main parses the input flags, creates a query, send the query to the server defined in the input, waits for a response and writes the result to the command line.
func main() {
	flag.Parse()
	if *revLookup != "" {
		//TODO CFE implement reverse lookup
		fmt.Println("TODO CFE reverse lookup is not yet supported")
	} else {
		switch flag.NArg() {
		case 0:
			//all information present
		case 2:
			serverAddr = &flag.Args()[0]
			name = &flag.Args()[1]
		case 3:
			serverAddr = &flag.Args()[0]
			name = &flag.Args()[1]
			typeNo, err := strconv.Atoi(flag.Args()[2])
			if err != nil {
				fmt.Println("malformed type")
				os.Exit(1)
			}
			queryType = &typeNo
		default:
			fmt.Println("input parameters malformed")
		}

		tcpAddr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("%s:%d", *serverAddr, *port))
		if err != nil {
			fmt.Printf("serverAddr malformed, error=%v\n", err)
			os.Exit(1)
		}
		connInfo := connection.Info{Type: connection.TCP, TCPAddr: tcpAddr}

		var qt []object.Type
		if *queryType == -1 {
			qt = anyQuery
		} else {
			qt = []object.Type{object.Type(*queryType)}
		}

		msg := util.NewQueryMessage(*name, *context, *expires, qt, queryOptions, token.New())

		err = sendQuery(msg, msg.Token, connInfo)
		if err != nil {
			log.Info(fmt.Sprintf("could not send query: %v", err))
			os.Exit(1)
		}
	}
}

//sendQuery creates a connection with connInfo, frames msg and writes it to the connection.
//It then waits for the response which it then outputs to the command line and if specified additionally stores to a file.
func sendQuery(msg message.Message, token token.Token, connInfo connection.Info) error {
	conn, err := createConnection(connInfo)
	if err != nil {
		return err
	}
	defer conn.Close()

	done := make(chan message.Message)
	ec := make(chan error)
	go listen(conn, token, done, ec)

	writer := cbor.NewWriter(conn)
	if err := writer.Marshal(&msg); err != nil {
		return fmt.Errorf("failed to marshal message: %v", err)
	}

	select {
	case msg := <-done:
		for _, section := range msg.Content {
			// TODO: validate signatures.
			fmt.Println(zfParser.Encode(section))
		}
	case err := <-ec:
		return err
	case <-time.After(10 * time.Second):
		return fmt.Errorf("timed out waiting for response")
	}
	return nil
}

//createConnection returns a newly created connection with connInfo or an error
func createConnection(connInfo connection.Info) (conn net.Conn, err error) {
	switch connInfo.Type {
	case connection.TCP:
		return tls.Dial(connInfo.TCPAddr.Network(), connInfo.String(), &tls.Config{InsecureSkipVerify: *insecureTLS})
	default:
		return nil, errors.New("unsupported Network address type")
	}
}

func listen(conn net.Conn, tok token.Token, done chan<- message.Message, ec chan<- error) {
	reader := cbor.NewReader(conn)
	var msg message.Message
	if err := reader.Unmarshal(&msg); err != nil {
		if err.Error() == "failed to read tag: EOF" {
			ec <- fmt.Errorf("connection has been closed")
		} else {
			ec <- fmt.Errorf("failed to unmarshal response: %v", err)
		}
		return
	}
	if msg.Token != tok {
		if n, ok := msg.Content[0].(*section.Notification); !ok || n.Token != tok {
			ec <- fmt.Errorf("token response mismatch: got %v, want %v", msg.Token, tok)
			return
		}
	}
	done <- msg
}

//qoptFlag defines the query options flag. It allows a user to specify multiple query options and their priority (by input sequence)
type qoptFlag []query.Option

func (i *qoptFlag) String() string {
	list := []string{}
	for _, opt := range *i {
		list = append(list, strconv.Itoa(int(opt)))
	}
	return fmt.Sprintf("[%s]", strings.Join(list, " "))
}

//Set transforms command line input of a query option to its internal representation
func (i *qoptFlag) Set(value string) error {
	switch value {
	case "1":
		*i = append(*i, query.QOMinE2ELatency)
	case "2":
		*i = append(*i, query.QOMinLastHopAnswerSize)
	case "3":
		*i = append(*i, query.QOMinInfoLeakage)
	case "4":
		*i = append(*i, query.QOCachedAnswersOnly)
	case "5":
		*i = append(*i, query.QOExpiredAssertionsOk)
	case "6":
		*i = append(*i, query.QOTokenTracing)
	case "7":
		*i = append(*i, query.QONoVerificationDelegation)
	case "8":
		*i = append(*i, query.QONoProactiveCaching)
	default:
		return fmt.Errorf("There is no query option for value: %s", value)
	}
	return nil
}
