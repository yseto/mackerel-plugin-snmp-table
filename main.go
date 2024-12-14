package main

import (
	"bytes"
	"cmp"
	"context"
	"flag"
	"fmt"
	"log"
	"maps"
	"os"
	"os/signal"
	"slices"
	"strconv"
	"strings"
	"text/template"

	mackerelplugin "github.com/mackerelio/go-mackerel-plugin"
	"github.com/olekukonko/tablewriter"
	"github.com/sleepinggenius2/gosmi/types"
	"gopkg.in/yaml.v3"

	"github.com/yseto/mackerel-plugin-snmp-table/config"
	"github.com/yseto/mackerel-plugin-snmp-table/smi"
	"github.com/yseto/mackerel-plugin-snmp-table/snmp"
)

var (
	configFilename string
	preview        bool
)

func init() {
	flag.StringVar(&configFilename, "conf", "config.yml", "config `filename`")
	flag.BoolVar(&preview, "preview", false, "show table")
}

type Column struct {
	Name string
	Oid  string
}

func main() {
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	f, err := os.ReadFile(configFilename)
	if err != nil {
		log.Fatalln(err)
	}

	var conf config.Config
	err = yaml.Unmarshal(f, &conf)
	if err != nil {
		log.Fatalf("error: %v", err)
	}

	mibParser := smi.New(conf.MIB.LoadModules, conf.MIB.Directory)
	if err := mibParser.Init(); err != nil {
		log.Println(err)
	}
	defer mibParser.Close()

	oid, mibs, err := mibParser.FromOID(conf.Target.Oid)
	if err != nil {
		log.Println(err)
	}

	// MIBの解析結果から、項目を抽出する
	var index string
	var columns = make(map[uint64]Column, 0) // uintで map してるのは、ソートの結果がまあまあ綺麗になるからで、自然順ソートすればこれは何でもよい
	for _, mib := range mibs {
		if mib.Kind == types.NodeRow {
			mibIndex := mib.GetIndex()

			// TODO インデックスは複数許容される仕様のようだが、単一のインデックスしかサポートできていない
			if len(mibIndex) == 0 {
				log.Fatalf("row %s is not have index", mib.Oid.String())
			}

			// eg. 1.3.6.1.2.1.2.2.1.1
			index = mibIndex[0].Oid.String()
		}
		if mib.Kind == types.NodeColumn {
			number, err := captureIndex(mib.Oid.String())
			if err != nil {
				log.Fatal(err)
			}
			columns[number] = Column{Name: mib.Name, Oid: mib.Oid.String()}
		}
	}

	// SNMP の接続
	s, err := snmp.Init(ctx, conf.Target.IPAddress, conf.Target.Community)
	if err != nil {
		log.Fatal(err)
	}

	pdus, err := s.BilkWalk(oid, mibs)
	if err != nil {
		log.Fatal(err)
	}

	// 得られた結果の key から index に利用可能なデータを取り出し、添え字を抽出する
	var indexes []uint64
	for key := range maps.Keys(pdus) {
		if strings.HasPrefix(key, "."+index+".") {
			i, err := captureIndex(key)
			if err != nil {
				log.Fatal(err)
			}
			indexes = append(indexes, i)
		}
	}

	slices.Sort(indexes)

	if preview {
		// output table view
		table := tablewriter.NewWriter(os.Stdout)

		var header []string
		for _, col := range slices.Sorted(maps.Keys(columns)) {
			header = append(header, columns[col].Name)
		}
		table.SetHeader(header)

		for _, row := range indexes {
			var line []string
			for _, col := range slices.Sorted(maps.Keys(columns)) {
				r := fmt.Sprintf(".%s.%d", columns[col].Oid, row)
				line = append(line, pdus[r])
			}
			table.Append(line)
		}
		table.SetBorders(tablewriter.Border{Left: true, Top: false, Right: true, Bottom: false})
		table.SetColumnSeparator("")
		table.SetAutoFormatHeaders(false)
		table.SetHeaderLine(false)
		table.Render()
	}

	// fmt.Printf("%+v\n", conf.Metric)

	var defs []GraphDef
	var metrics = make(map[string]float64, 0)

	for _, metric := range conf.Metric {

		// グラフ定義を生成
		for _, metricValue := range metric.Value {
			// TODO 色々増やしたくなったら検討する
			pad := make(map[string]string)
			pad["MetricValue"] = metricValue.Name

			tmpl := template.Must(template.New("prefix").Parse(metric.Prefix))
			var wr bytes.Buffer
			if err := tmpl.Execute(&wr, pad); err != nil {
				log.Fatalln(err)
			}

			defs = append(defs, GraphDef{
				Prefix: wr.String(),
				Label:  metricValue.Label,
				Unit:   cmp.Or(metricValue.Unit, "float"),
				Diff:   metricValue.Diff,
			})
		}

		// メトリック名のテンプレート
		tmplStr := fmt.Sprintf("%s.%s", metric.Prefix, metric.Key)
		tmpl := template.Must(template.New("key").Parse(tmplStr))

		// 各行ごとに処理
		for _, row := range indexes {
			pad := make(map[string]string)

			for _, col := range slices.Sorted(maps.Keys(columns)) {
				r := fmt.Sprintf(".%s.%d", columns[col].Oid, row)
				pad[columns[col].Name] = pdus[r]
			}

			for _, metricValue := range metric.Value {
				pad["MetricValue"] = metricValue.Name

				var wr bytes.Buffer
				if err := tmpl.Execute(&wr, pad); err != nil {
					log.Fatalln(err)
				}

				// fmt.Println(wr.String(), pad[metricValue.Name])

				val, err := strconv.ParseFloat(pad[metricValue.Name], 64)
				if err != nil {
					log.Fatalln(err)
				}
				metrics[wr.String()] = val
			}
		}
	}

	var plugin = MP{
		Prefix:    conf.Prefix,
		Metrics:   metrics,
		GraphDefs: defs,
	}

	helper := mackerelplugin.NewMackerelPlugin(plugin)
	helper.Run()
}

func captureIndex(name string) (uint64, error) {
	sl := strings.Split(name, ".")
	return strconv.ParseUint(sl[len(sl)-1], 10, 64)
}

type GraphDef struct {
	Prefix string
	Label  string
	Unit   string
	Diff   bool
}

type MP struct {
	Prefix    string
	Metrics   map[string]float64
	GraphDefs []GraphDef
}

func (m MP) MetricKeyPrefix() string {
	return cmp.Or(m.Prefix, "snmp-table")
}

func (m MP) FetchMetrics() (map[string]float64, error) {
	return m.Metrics, nil
}

func (m MP) GraphDefinition() map[string]mackerelplugin.Graphs {
	var graphdef = make(map[string]mackerelplugin.Graphs, 0)
	for _, def := range m.GraphDefs {
		graphdef[def.Prefix] = mackerelplugin.Graphs{
			Label:   def.Label,
			Unit:    def.Unit,
			Metrics: []mackerelplugin.Metrics{{Name: "*", Label: "%1", Diff: def.Diff}},
		}
	}

	return graphdef
}
