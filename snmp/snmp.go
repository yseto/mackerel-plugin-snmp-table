package snmp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gosnmp/gosnmp"
	"github.com/sleepinggenius2/gosmi"
)

type Handler interface {
	BulkWalk(rootOid string, walkFn gosnmp.WalkFunc) error

	Connect() error
	Close() error
}

type snmpHandler struct {
	gosnmp.GoSNMP
}

func NewHandler(ctx context.Context, target, community string) Handler {
	return &snmpHandler{
		gosnmp.GoSNMP{
			Context:            ctx,
			Target:             target,
			Port:               161,
			Transport:          "udp",
			Community:          community,
			Version:            gosnmp.Version2c,
			Timeout:            time.Duration(10) * time.Second,
			Retries:            3,
			ExponentialTimeout: true,
			MaxOids:            gosnmp.MaxOids,
		},
	}
}

func (x *snmpHandler) Close() error {
	return x.GoSNMP.Conn.Close()
}

type SNMP struct {
	handler Handler
}

func Init(ctx context.Context, target, community string) (*SNMP, error) {
	g := NewHandler(ctx, target, community)
	err := g.Connect()
	if err != nil {
		return nil, err
	}
	return &SNMP{handler: g}, nil
}

func (s *SNMP) Close() error {
	return s.handler.Close()
}

func (s *SNMP) BilkWalk(oid string, smis []gosmi.SmiNode) (map[string]string, error) {
	kv := make(map[string]string, 0)
	err := s.handler.BulkWalk(oid, func(pdu gosnmp.SnmpPDU) error {
		index := pdu.Name

		// OctetString のパースをするのに、書式を得る
		var format string
		for _, smi := range smis {
			// ループでとりあえず対処している。いい方法があったら書き直したい
			suffix := "." + smi.Oid.String() + "."
			if strings.HasPrefix(index, suffix) {
				if smi.Type != nil {
					format = smi.Type.Format
				}
			}
		}

		switch pdu.Type {
		case gosnmp.OctetString:
			// TODO 色々な書式のサポート
			if format == "1x:" {
				value, ok := pdu.Value.([]byte)
				if !ok {
					return fmt.Errorf("can not parse : %s", value)
				}
				var parts []string
				for _, i := range value {
					// %02x は見た目が整ってるからいいな。くらいであって、これが適当かはわからない
					parts = append(parts, fmt.Sprintf("%02x", i))
				}
				kv[index] = strings.Join(parts, ":")
			} else {
				kv[index] = string(pdu.Value.([]byte))
			}
		default:
			kv[index] = gosnmp.ToBigInt(pdu.Value).String()
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return kv, nil
}
