package smi

import (
	"fmt"
	"strings"

	"github.com/sleepinggenius2/gosmi"
	"github.com/sleepinggenius2/gosmi/types"
)

type SMI struct {
	Modules []string
	Paths   []string
}

func New(modules, paths []string) *SMI {
	return &SMI{
		Modules: modules,
		Paths:   paths,
	}
}

func (s *SMI) Init() error {
	gosmi.Init()

	for _, path := range s.Paths {
		gosmi.AppendPath(path)
	}
	for i, module := range s.Modules {
		moduleName, err := gosmi.LoadModule(module)
		if err != nil {
			return err
		}
		s.Modules[i] = moduleName
	}
	return nil
}

func (s *SMI) Close() {
	gosmi.Exit()
}

// 値のパース、検証
func (s *SMI) FromOID(oid string) (string, []gosmi.SmiNode, error) {
	var node gosmi.SmiNode
	var err error
	if (oid[0] >= '0' && oid[0] <= '9') || oid[0] == '.' { // eg. .1.3.6.1.2.1.2.2
		node, err = gosmi.GetNodeByOID(types.OidMustFromString(oid))

	} else if strings.Contains(oid, "::") { // eg. IF-MIB::ifTable
		params := strings.SplitN(oid, "::", 2)

		var module gosmi.SmiModule
		module, err = gosmi.GetModule(params[0])
		if err != nil {
			return "", nil, err
		}
		node, err = gosmi.GetNode(params[1], module)

	} else {
		return "", nil, fmt.Errorf("not found : %s", oid)
	}

	if err != nil {
		return "", nil, err
	}

	// テーブルであることを期待する
	if node.Kind != types.NodeTable {
		return "", nil, fmt.Errorf("oid is not Table : %s", node.Kind)
	}

	return node.Oid.String(), node.GetSubtree(), nil
}
