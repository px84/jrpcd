# jrpcd
Generic JSON RPC router with caching support

Syntax:
```
./jrpcd \
  --listen 127.0.0.1:9545 \
  --cachedir /tmp/cache \
  --logfile /tmp/jrpcd.log \
  --loglevel debug \
  -b http://localhost:8545/ \
  -b someid=https://some.node:8545/with/trace/support/MY-SECRET-ID \
  -r "someid=match:trace_*"
 ```
  This starts a JSON RPC router waiting for requests on 127.0.0.1:9545. As cachedir is specified, responses from upstream servers are
  cached in the given directory and served from there for subsequent requests. Logs are stored in the given file, its contents will be
  rotated automatically according to internal hardcoded rules, currently limiting it to 10MB size and 5 backups kept for max. 7 days.
  A local JSON RPC backend at port 8545 is added. It has no explicit name ($name=URL), so it gets the name "default". The default backend
  will be used for all requests that do not match a specific rule.
  Another remote backend is added with the name "someid".
  A routing rule is added. All requests whose method matches the trace_* pattern will be routed to the someid backend.
 
