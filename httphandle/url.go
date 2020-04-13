package httphandle

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"go.etcd.io/etcd/clientv3"
)

// etcd://[username:password@]host1:port1[,...hostN:portN][/[defaultPrefix][?options]]
type urlInfoOption struct {
	key   string
	value string
}

type urlInfo struct {
	addrs   []string
	user    string
	pass    string
	prefix  string
	options []urlInfoOption
}

func isOptSep(c rune) bool {
	return c == ';' || c == '&'
}

func extractURL(s string) (*urlInfo, error) {
	s = strings.TrimPrefix(s, "etcd://")
	info := &urlInfo{
		prefix:  "\x00",
		options: []urlInfoOption{},
	}

	if c := strings.Index(s, "?"); c != -1 {
		for _, pair := range strings.FieldsFunc(s[c+1:], isOptSep) {
			l := strings.SplitN(pair, "=", 2)
			if len(l) != 2 || l[0] == "" || l[1] == "" {
				return nil, errors.New("connection option must be key=value: " + pair)
			}
			info.options = append(info.options, urlInfoOption{key: l[0], value: l[1]})
		}
		s = s[:c]
	}
	if c := strings.Index(s, "@"); c != -1 {
		pair := strings.SplitN(s[:c], ":", 2)
		if len(pair) > 2 || pair[0] == "" {
			return nil, errors.New("credentials must be provided as user:pass@host")
		}
		var err error
		info.user, err = url.QueryUnescape(pair[0])
		if err != nil {
			return nil, fmt.Errorf("cannot unescape username in URL: %q", pair[0])
		}
		if len(pair) > 1 {
			info.pass, err = url.QueryUnescape(pair[1])
			if err != nil {
				return nil, fmt.Errorf("cannot unescape password in URL")
			}
		}
		s = s[c+1:]
	}

	if c := strings.Index(s, ","); c != -1 {
		addrs := strings.Split(s, ",")
		for i := 0; i < len(addrs)-1; i++ {
			addr := addrs[i]
			if !strings.HasPrefix(addr, "http://") {
				addr = "http://" + addr
			}
			info.addrs = append(info.addrs, addr)
		}
		s = strings.TrimLeft(addrs[len(addrs)-1], "http://")
	}

	if c := strings.Index(s, "/"); c != -1 {
		info.prefix = s[c:]
		s = s[:c]
	}
	info.addrs = append(info.addrs, "http://"+s)
	return info, nil
}

func parseURL(url string) (string, clientv3.Config, error) {
	uinfo, err := extractURL(url)
	cfg := clientv3.Config{
		Endpoints:           uinfo.addrs,
		Username:            uinfo.user,
		Password:            uinfo.pass,
		PermitWithoutStream: true,
		DialTimeout:         3 * time.Second,
	}
	if err != nil {
		return "", cfg, err
	}

	for _, opt := range uinfo.options {
		switch opt.key {
		case "auto-sync-interval":
			n, err := strconv.ParseInt(opt.value, 10, 32)
			if err != nil {
				return "", cfg, errors.New("auto-sync-interval parse int " + err.Error())
			}
			cfg.AutoSyncInterval = time.Duration(n) * time.Second
		case "dial-timeout":
			n, err := strconv.ParseInt(opt.value, 10, 32)
			if err != nil {
				return "", cfg, errors.New("dial-timeout parse int " + err.Error())
			}
			cfg.DialTimeout = time.Duration(n) * time.Second
		case "dial-keep-alive-time":
			n, err := strconv.ParseInt(opt.value, 10, 32)
			if err != nil {
				return "", cfg, errors.New("dial-keep-alive-time parse int " + err.Error())
			}
			cfg.DialKeepAliveTime = time.Duration(n) * time.Second
		case "dial-keep-alive-timeout":
			n, err := strconv.ParseInt(opt.value, 10, 32)
			if err != nil {
				return "", cfg, errors.New("dial-keep-alive-timeout parse int " + err.Error())
			}
			cfg.DialKeepAliveTimeout = time.Duration(n) * time.Second
		case "max-send-msg-size":
			n, err := strconv.ParseInt(opt.value, 10, 32)
			if err != nil {
				return "", cfg, errors.New("max-send-msg-size parse int " + err.Error())
			}
			cfg.MaxCallSendMsgSize = int(n) * 1024
		case "max-recv-msg-size":
			n, err := strconv.ParseInt(opt.value, 10, 32)
			if err != nil {
				return "", cfg, errors.New("max-recv-msg-size parse int " + err.Error())
			}
			cfg.MaxCallRecvMsgSize = int(n) * 1024
		}
	}

	return uinfo.prefix, cfg, nil
}
