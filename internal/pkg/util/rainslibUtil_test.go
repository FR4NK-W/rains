package util

import (
	"reflect"
	"testing"
	"time"

	"github.com/netsec-ethz/rains/internal/pkg/message"

	"github.com/netsec-ethz/rains/internal/pkg/object"
	"github.com/netsec-ethz/rains/internal/pkg/query"
	"github.com/netsec-ethz/rains/internal/pkg/section"
	"github.com/netsec-ethz/rains/internal/pkg/token"
)

const (
	testSubjectName = "test"
	testZone        = "zone"
	globalContext   = "context"
	ip4TestAddr     = "192.0.2.0"
)

func TestSaveAndLoad(t *testing.T) {
	var tests = []struct {
		input       *section.Assertion
		output      *section.Assertion
		path        string
		storeErrMsg string
		loadErrMsg  string
	}{
		{&section.Assertion{SubjectName: testSubjectName, SubjectZone: testZone, Context: globalContext, Content: []object.Object{object.Object{Type: object.OTIP4Addr, Value: ip4TestAddr}}},
			new(section.Assertion), "test/test.gob", "", ""},
		{&section.Assertion{SubjectName: testSubjectName, SubjectZone: testZone, Context: globalContext, Content: []object.Object{object.Object{Type: object.OTIP4Addr, Value: ip4TestAddr}}},
			nil, "test/test.gob", "", "gob: DecodeValue of unassignable value"},
		{&section.Assertion{SubjectName: testSubjectName, SubjectZone: "ch", Context: globalContext, Content: []object.Object{object.Object{Type: object.OTIP4Addr, Value: ip4TestAddr}}},
			new(section.Assertion), "nonExistDir/test.gob", "open nonExistDir/test.gob: no such file or directory", "open nonExistDir/test.gob: no such file or directory"},
	}
	for i, test := range tests {
		err := Save(test.path, test.input)
		if err != nil && err.Error() != test.storeErrMsg {
			t.Errorf("%d: Was not able to save data and error msgs do notmatch. expected=%s actual=%s", i, test.storeErrMsg, err.Error())
		}
		err = Load(test.path, test.output)
		if err != nil && err.Error() != test.loadErrMsg {
			t.Errorf("%d: Was not able to load data and error msgs do notmatch. expected=%s actual=%s", i, test.loadErrMsg, err.Error())
		}
		if err == nil && !reflect.DeepEqual(test.output, test.input) {
			t.Errorf("%d: Loaded object has different value. expected=%v actual=%v", i, test.input, test.output)
		}
	}
}

func TestUpdateSectionValidity(t *testing.T) {
	now := time.Now().Unix()
	var tests = []struct {
		input          section.WithSigForward
		pkeyValidSince int64
		pkeyValidUntil int64
		sigValidSince  int64
		sigValidUntil  int64
		maxVal         MaxCacheValidity
		wantValidSince int64
		wantValidUntil int64
	}{
		{input: nil}, //should not result in panic
		{new(section.Assertion), now + 1, now + 4, now + 2, now + 3, MaxCacheValidity{AssertionValidity: 4 * time.Second}, now + 2, now + 3},
		{new(section.Assertion), now + 2, now + 3, now + 1, now + 4, MaxCacheValidity{AssertionValidity: 4 * time.Second}, now + 2, now + 3},
		{new(section.Assertion), now + 1, now + 3, now + 2, now + 4, MaxCacheValidity{AssertionValidity: 4 * time.Second}, now + 2, now + 3},
		{new(section.Assertion), now + 2, now + 4, now + 1, now + 3, MaxCacheValidity{AssertionValidity: 4 * time.Second}, now + 2, now + 3},
		{new(section.Assertion), now + 2, now + 4, now + 1, now + 3, MaxCacheValidity{AssertionValidity: 2 * time.Second}, now + 2, now + 2},
		{new(section.Assertion), now + 2, now + 4, now + 1, now + 3, MaxCacheValidity{AssertionValidity: 1 * time.Second}, now + 1, now + 1},

		{new(section.Shard), now + 1, now + 4, now + 2, now + 3, MaxCacheValidity{ShardValidity: 4 * time.Second}, now + 2, now + 3},
		{new(section.Shard), now + 2, now + 3, now + 1, now + 4, MaxCacheValidity{ShardValidity: 4 * time.Second}, now + 2, now + 3},
		{new(section.Shard), now + 1, now + 3, now + 2, now + 4, MaxCacheValidity{ShardValidity: 4 * time.Second}, now + 2, now + 3},
		{new(section.Shard), now + 2, now + 4, now + 1, now + 3, MaxCacheValidity{ShardValidity: 4 * time.Second}, now + 2, now + 3},
		{new(section.Shard), now + 2, now + 4, now + 1, now + 3, MaxCacheValidity{ShardValidity: 2 * time.Second}, now + 2, now + 2},
		{new(section.Shard), now + 2, now + 4, now + 1, now + 3, MaxCacheValidity{ShardValidity: 1 * time.Second}, now + 1, now + 1},

		{new(section.Zone), now + 1, now + 4, now + 2, now + 3, MaxCacheValidity{ZoneValidity: 4 * time.Second}, now + 2, now + 3},
		{new(section.Zone), now + 2, now + 3, now + 1, now + 4, MaxCacheValidity{ZoneValidity: 4 * time.Second}, now + 2, now + 3},
		{new(section.Zone), now + 1, now + 3, now + 2, now + 4, MaxCacheValidity{ZoneValidity: 4 * time.Second}, now + 2, now + 3},
		{new(section.Zone), now + 2, now + 4, now + 1, now + 3, MaxCacheValidity{ZoneValidity: 4 * time.Second}, now + 2, now + 3},
		{new(section.Zone), now + 2, now + 4, now + 1, now + 3, MaxCacheValidity{ZoneValidity: 2 * time.Second}, now + 2, now + 2},
		{new(section.Zone), now + 2, now + 4, now + 1, now + 3, MaxCacheValidity{ZoneValidity: 1 * time.Second}, now + 1, now + 1},
	}
	for i, test := range tests {
		UpdateSectionValidity(test.input, test.pkeyValidSince, test.pkeyValidUntil, test.sigValidSince, test.sigValidUntil, test.maxVal)
		if test.input != nil && test.input.ValidSince() != test.wantValidSince {
			t.Errorf("%d: ValidSince does not match. expected=%d actual=%d", i, test.wantValidSince, test.input.ValidSince())
		}
		if test.input != nil && test.input.ValidUntil() != test.wantValidUntil {
			t.Errorf("%d: ValidUntil does not match. expected=%d actual=%d", i, test.wantValidUntil, test.input.ValidUntil())
		}
	}
}

func TestNewQueryMessage(t *testing.T) {
	tok := token.New()
	var tests = []struct {
		context  string
		name     string
		expires  int64
		types    []object.Type
		options  []query.Option
		token    token.Token
		expected message.Message
	}{
		{".", "example.com", 100, []object.Type{object.OTIP4Addr}, []query.Option{query.QOTokenTracing, query.QOMinE2ELatency}, tok,
			message.Message{
				Token: tok,
				Content: []section.Section{
					&query.Name{
						Name:       "example.com",
						Context:    ".",
						Expiration: 100,
						Types:      []object.Type{object.OTIP4Addr},
						Options:    []query.Option{query.QOTokenTracing, query.QOMinE2ELatency},
					},
				},
			},
		},
	}
	for i, test := range tests {
		msg := NewQueryMessage(test.name, test.context, test.expires, test.types, test.options, test.token)
		if !reflect.DeepEqual(test.expected, msg) {
			t.Errorf("%d: Message containing Query do not match. expected=%v actual=%v", i, test.expected, msg)
		}
	}
}

func TestNewNotificationsMessage(t *testing.T) {
	tokens := []token.Token{}
	for i := 0; i < 10; i++ {
		tokens = append(tokens, token.New())
	}
	var tests = []struct {
		tokens   []token.Token
		types    []section.NotificationType
		data     []string
		expected message.Message
		errMsg   string
	}{
		{tokens[:2], []section.NotificationType{section.NTHeartbeat, section.NTMsgTooLarge}, []string{"1", "2"},
			message.Message{Content: []section.Section{&section.Notification{Token: tokens[0], Type: section.NTHeartbeat, Data: "1"},
				&section.Notification{Token: tokens[1], Type: section.NTMsgTooLarge, Data: "2"}}}, ""},
		{tokens[:3], []section.NotificationType{section.NTHeartbeat, section.NTMsgTooLarge}, []string{"1", "2"}, message.Message{}, "input slices have not the same length"},
	}
	for i, test := range tests {
		msg, err := NewNotificationsMessage(test.tokens, test.types, test.data)
		test.expected.Token = msg.Token
		if err == nil && !reflect.DeepEqual(test.expected, msg) {
			t.Errorf("%d: Message containing Notifications do not match. expected=%v actual=%v", i, test.expected, msg)
		}
		if err != nil && err.Error() != test.errMsg {
			t.Errorf("%d: error msg do not match. expected=%v actual=%v", i, test.errMsg, err.Error())
		}
	}
}

func TestNewNotificationMessage(t *testing.T) {
	tok := token.New()
	var tests = []struct {
		token    token.Token
		t        section.NotificationType
		data     string
		expected message.Message
	}{
		{tok, section.NTHeartbeat, "1",
			message.Message{Content: []section.Section{&section.Notification{Token: tok, Type: section.NTHeartbeat, Data: "1"}}}},
	}
	for i, test := range tests {
		msg := NewNotificationMessage(test.token, test.t, test.data)
		test.expected.Token = msg.Token
		if !reflect.DeepEqual(test.expected, msg) {
			t.Errorf("%d: Message containing Notification do not match. expected=%v actual=%v", i, test.expected, msg)
		}
	}
}
