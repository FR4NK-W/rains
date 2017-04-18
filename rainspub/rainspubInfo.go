package rainspub

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"net"
	"rains/rainsd"
	"rains/rainslib"
	"strconv"
	"time"

	log "github.com/inconshreveable/log15"
)

const (
	configPath = "config/rainspub.conf"
)

//Config contains configurations for publishing assertions
var config = defaultConfig
var parser zoneFileParser
var msgParser rainslib.RainsMsgParser

//rainpubConfig lists configurations for publishing zone information
type rainpubConfig struct {
	assertionValidity     time.Duration
	shardValidity         time.Duration
	zoneValidity          time.Duration
	maxAssertionsPerShard uint
	serverAddresses       []rainsd.ConnInfo
	zoneFilePath          string
}

//DefaultConfig is a rainpubConfig object containing default values
var defaultConfig = rainpubConfig{assertionValidity: 30 * 24 * time.Hour, shardValidity: 24 * time.Hour, zoneValidity: 24 * time.Hour, maxAssertionsPerShard: 5,
	serverAddresses: []rainsd.ConnInfo{rainsd.ConnInfo{Type: rainsd.TCP, IPAddr: net.ParseIP("127.0.0.1"), Port: 5022}}}

type zoneFileParser interface {
	//parseZoneFile takes as input a zoneFile and returns all contained assertions. A zoneFile has the following format:
	//:Z: <context> <zone> [(:S:<Shard Content>|:A:<Assertion Content>)*]
	//Shard Content: [(:A:<Assertion Content>)*]
	//Assertion Content: <subject-name>[(:objectType:<object data>)*]
	//It assumes that
	parseZoneFile(zoneFile []byte) ([]*rainslib.AssertionSection, error)
}

type zoneFileParserImpl struct {
}

func (p zoneFileParserImpl) parseZoneFile(zoneFile []byte) ([]*rainslib.AssertionSection, error) {
	assertions := []*rainslib.AssertionSection{}
	scanner := bufio.NewScanner(bytes.NewReader(zoneFile))
	scanner.Split(bufio.ScanWords)
	scanner.Scan()
	if scanner.Text() != ":Z:" {
		log.Warn("zoneFile malformed. It does not start with :Z:")
	}
	scanner.Scan()
	context := scanner.Text()
	scanner.Scan()
	zone := scanner.Text()
	scanner.Scan() //scan [
	scanner.Scan()
	for scanner.Text() != "]" {
		switch scanner.Text() {
		case ":A:":
			a, err := parseAssertion(context, zone, scanner)
			if err != nil {
				return nil, err
			}
			assertions = append(assertions, a)
		case ":S:":
			asserts, err := parseShard(context, zone, scanner)
			if err != nil {
				return nil, err
			}
			assertions = append(assertions, asserts...)
		default:
			return nil, fmt.Errorf("Expected a shard or assertion inside the zone but got=%s", scanner.Text())
		}
		scanner.Scan()
	}
	return assertions, nil
}

func parseShard(context, zone string, scanner *bufio.Scanner) ([]*rainslib.AssertionSection, error) {
	assertions := []*rainslib.AssertionSection{}
	scanner.Scan() //scans [
	scanner.Scan()
	for scanner.Text() != "]" {
		if scanner.Text() != ":A:" {
			return nil, fmt.Errorf("zone file malformated. Expected Assertion inside shard but got=%s", scanner.Text())
		}
		a, err := parseAssertion(context, zone, scanner)
		if err != nil {
			return nil, err
		}
		assertions = append(assertions, a)
		scanner.Scan()
	}
	return assertions, nil
}

//parseAssertion parses the assertions content and returns an assertion section
func parseAssertion(context, zone string, scanner *bufio.Scanner) (*rainslib.AssertionSection, error) {
	scanner.Scan()
	name := scanner.Text()
	scanner.Scan() //scans [
	scanner.Scan()
	objects := []rainslib.Object{}
	for scanner.Text() != "]" {
		switch scanner.Text() {
		case ":name:":
			scanner.Scan()
			objects = append(objects, rainslib.Object{Type: rainslib.OTName, Value: scanner.Text()})
		case ":ip6:":
			scanner.Scan()
			objects = append(objects, rainslib.Object{Type: rainslib.OTIP6Addr, Value: scanner.Text()})
		case ":ip4:":
			scanner.Scan()
			objects = append(objects, rainslib.Object{Type: rainslib.OTIP4Addr, Value: scanner.Text()})
		case ":redir:":
			scanner.Scan()
			objects = append(objects, rainslib.Object{Type: rainslib.OTRedirection, Value: scanner.Text()})
		case ":deleg:":
			//TODO CFE more complex contains public key
		case ":nameset:":
			scanner.Scan()
			objects = append(objects, rainslib.Object{Type: rainslib.OTNameset, Value: scanner.Text()})
		case ":cert:":
			cert, err := getCertObject(scanner)
			if err != nil {
				return nil, err
			}
			objects = append(objects, rainslib.Object{Type: rainslib.OTCertInfo, Value: cert})
		case ":srv:":
			srvInfo := rainslib.ServiceInfo{}
			scanner.Scan()
			srvInfo.Name = scanner.Text()
			scanner.Scan()
			portNr, err := strconv.Atoi(scanner.Text())
			if err != nil {
				return nil, err
			}
			srvInfo.Port = uint16(portNr)
			scanner.Scan()
			prio, err := strconv.Atoi(scanner.Text())
			if err != nil {
				return nil, err
			}
			srvInfo.Priority = uint(prio)
			objects = append(objects, rainslib.Object{Type: rainslib.OTServiceInfo, Value: srvInfo})
		case ":regr:":
			scanner.Scan()
			objects = append(objects, rainslib.Object{Type: rainslib.OTRegistrar, Value: scanner.Text()})
		case ":regt:":
			scanner.Scan()
			objects = append(objects, rainslib.Object{Type: rainslib.OTRegistrant, Value: scanner.Text()})
		case ":infra:":
			//TODO CFE more complex contains public key
		case ":extra:":
			//TODO CFE more complex contains public key
		default:
			return nil, errors.New("Encountered non existing object type")
		}
		scanner.Scan() //scan next object type
	}
	return &rainslib.AssertionSection{Context: context, SubjectZone: zone, SubjectName: name, Content: objects}, nil
}

func getCertObject(scanner *bufio.Scanner) (rainslib.CertificateObject, error) {
	scanner.Scan()
	certType, err := getCertPT(scanner.Text())
	if err != nil {
		return rainslib.CertificateObject{}, err
	}
	scanner.Scan()
	usage, err := getCertUsage(scanner.Text())
	if err != nil {
		return rainslib.CertificateObject{}, err
	}
	scanner.Scan()
	hashAlgo, err := getCertHashType(scanner.Text())
	if err != nil {
		return rainslib.CertificateObject{}, err
	}
	scanner.Scan()
	cert := rainslib.CertificateObject{
		Type:     certType,
		Usage:    usage,
		HashAlgo: hashAlgo,
		Data:     scanner.Bytes(),
	}
	return cert, nil
}

func getCertPT(certType string) (rainslib.ProtocolType, error) {
	switch certType {
	case "0":
		return rainslib.PTUnspecified, nil
	case "1":
		return rainslib.PTTLS, nil
	default:
		return rainslib.ProtocolType(-1), errors.New("Encountered non existing certificate protocol type")
	}
}

func getCertUsage(certType string) (rainslib.CertificateUsage, error) {
	switch certType {
	case "2":
		return rainslib.CUTrustAnchor, nil
	case "3":
		return rainslib.CUEndEntity, nil
	default:
		return rainslib.CertificateUsage(-1), errors.New("Encountered non existing certificate usage")
	}
}

func getCertHashType(certType string) (rainslib.HashAlgorithmType, error) {
	switch certType {
	case "0":
		return rainslib.NoHashAlgo, nil
	case "1":
		return rainslib.Sha256, nil
	case "2":
		return rainslib.Sha384, nil
	case "3":
		return rainslib.Sha512, nil
	default:
		return rainslib.HashAlgorithmType(-1), errors.New("Encountered non existing certificate hash algorithm")
	}
}
