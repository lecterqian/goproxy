package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/op/go-logging"
	"github.com/shell909090/goproxy/cryptconn"
	"github.com/shell909090/goproxy/dns"
	"github.com/shell909090/goproxy/ipfilter"
	"github.com/shell909090/goproxy/msocks"
	"github.com/shell909090/goproxy/sutils"
	stdlog "log"
	"net"
	"net/http"
	"os"
)

var log = logging.MustGetLogger("")

type Config struct {
	Mode   string
	Listen string
	Server string

	Logfile  string
	Loglevel string

	Cipher     string
	Key        string
	Blackfile  string
	ResolvConf string

	Username string
	Password string
	Auth     map[string]string
}

func run_server(cfg *Config) (err error) {
	listener, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		return
	}

	listener, err = cryptconn.NewListener(
		listener, cfg.Cipher, cfg.Key)
	if err != nil {
		return
	}

	s, err := msocks.NewService(cfg.Auth, sutils.DefaultTcpDialer)
	if err != nil {
		return
	}

	return s.Serve(listener)
}

func load_resolv_conf(cfg *Config) (err error) {
	if cfg.ResolvConf != "" {
		err = dns.LoadConfig(cfg.ResolvConf)
		return
	}
	err = dns.LoadConfig("resolv.conf")
	if err == nil {
		return
	}
	err = dns.LoadConfig("/etc/goproxy/resolv.conf")
	return
}

func run_httproxy(cfg *Config) (err error) {
	err = load_resolv_conf(cfg)
	if err != nil {
		return
	}

	var dialer sutils.Dialer
	dialer, err = cryptconn.NewDialer(
		sutils.DefaultTcpDialer, cfg.Cipher, cfg.Key)
	if err != nil {
		return
	}

	dialer, err = msocks.NewDialer(
		dialer, cfg.Server, cfg.Username, cfg.Password)
	if err != nil {
		return
	}
	ndialer := dialer.(*msocks.Dialer)

	if cfg.Blackfile != "" {
		dialer, err = ipfilter.NewFilteredDialer(
			dialer, sutils.DefaultTcpDialer, cfg.Blackfile)
		if err != nil {
			return
		}
	}

	mux := http.NewServeMux()
	NewMsocksManager(ndialer).Register(mux)
	return http.ListenAndServe(cfg.Listen, NewProxy(dialer, mux))
}

func LoadConfig() (cfg Config, err error) {
	var configfile string
	flag.StringVar(&configfile, "config",
		"/etc/goproxy/config.json", "config file")
	flag.Parse()

	file, err := os.Open(configfile)
	if err != nil {
		return
	}
	defer file.Close()

	dec := json.NewDecoder(file)
	err = dec.Decode(&cfg)
	if err != nil {
		return
	}
	return
}

func SetLogging(cfg Config) (err error) {
	var file *os.File
	file = os.Stdout

	if cfg.Logfile != "" {
		file, err = os.OpenFile(cfg.Logfile,
			os.O_RDWR|os.O_CREATE|os.O_APPEND, 0600)
		if err != nil {
			log.Fatal(err)
		}
	}
	logBackend := logging.NewLogBackend(file, "",
		stdlog.LstdFlags|stdlog.Lmicroseconds|stdlog.Lshortfile)
	logging.SetBackend(logBackend)

	logging.SetFormatter(logging.MustStringFormatter("%{level}: %{message}"))

	lv, err := logging.LogLevel(cfg.Loglevel)
	if err != nil {
		panic(err.Error())
	}
	logging.SetLevel(lv, "")

	return
}

func main() {
	cfg, err := LoadConfig()
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	err = SetLogging(cfg)
	if err != nil {
		fmt.Println(err.Error())
		return
	}

	log.Notice("%s mode start.", cfg.Mode)
	switch cfg.Mode {
	case "stop":
		log.Info("server stopped in stop mode")
		return
	case "server":
		err = run_server(&cfg)
	case "http":
		err = run_httproxy(&cfg)
	default:
		log.Warning("not supported mode.")
	}
	if err != nil {
		log.Error("%s", err)
	}
	log.Info("server stopped")
}
