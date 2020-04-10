# etcdpad-core

etcdpad-core is [etcdpad-web](https://github.com/etcdpad/etcdpad-web) server which etcdv3 web ui.

* Support etcd 3.x only.
* The server use websocket protocol, support etcdv3 `query`, `create`, `delete`, `watch` operators.
* Support multiple etcdv3 instances to connect at the same time.
* Support multiple etcdv3 key prefixes connection multiplex the same etcdv3 instance.
* Support one connection operate some values change can push to others actively.
* `create` and `delete` operators will record log.

## Go Version

go1.14

## Usage 

```
Usage of ./etcdpad-core:
  -logpath string
        websocket client operate log file directory (default "/Users/xkey/workspace/go/src/etcdpad-core/oplog.log")
  -port int
        websocket listen port (default 8989)
  -stdout
        epad log output to stdout (default true)
```

## Connect DSN

```
etcd://[username:password@]host1:port1[,...hostN:portN][/[defaultPrefix][?options]]
```

| Component   | Description  |
| --------   | :-----:  |
| etcd://     | A required prefix to identify that this is a string in the standard connection format. |
| username:password@     | Optional. Authentication credentials. Username is a user name for authentication. Password is a password for authentication.|
| host[:port]     | The hosts of etcd servers |
| /defaultPrefix  | Optional. etcd prefix key, default '/' |
| ?options     | etcd clientv3 connect other configures  |

Connection Options

| Component   | Description  |
| --------   | :-----:  |
| auto-sync-interval     | 	 AutoSyncInterval is the interval to update endpoints with its latest members. 0 disables auto-sync. By default auto-sync is disabled. |
| dial-timeout  | DialTimeout is the timeout for failing to establish a connection. |
| dial-keep-alive-time     | DialKeepAliveTime is the time after which client pings the server to see if transport is alive.  |
| dial-keep-alive-timeout     |  DialKeepAliveTimeout is the time that the client waits for a response for the keep-alive probe. If the response is not received in this time, the connection is closed. |
| max-send-msg-size | MaxCallSendMsgSize is the client-side request send limit in bytes. If 0, it defaults to 2.0 MiB (2 * 1024 * 1024). |
| max-recv-msg-size     | MaxCallRecvMsgSize is the client-side response receive limit.If 0, it defaults to "math.MaxInt32", because range response can easily exceed request send limits.  |

## License

GPLv3