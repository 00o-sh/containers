// transmission-init renders /config/settings.json from TRANSMISSION__*
// environment variables and then execs transmission-daemon.
//
// It is a shell-free replacement for apps/transmission's entrypoint.sh
// (bash + minijinja-cli). The env contract is identical: every key from
// the old defaults/settings.json.j2 template, same env names, same
// defaults, and rendering only happens when at least one TRANSMISSION__*
// variable is present. One deliberate fix: the old template emitted
// invalid JSON (unquoted strings like 0.0.0.0, dangling values for
// unset no-default keys like `"rpc-password": ,`) — verified against
// the real minijinja engine — so env-driven settings could never load.
// This shim emits typed, valid JSON, and omits no-default keys whose
// env is unset (transmission then applies its own internal default).
// Octal-flavored keys (rpc-socket-mode, umask) are emitted as strings
// to preserve leading zeros; transmission 4.x accepts both forms.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
)

var version = "dev"

type setting struct {
	key  string
	env  string
	typ  string // "int" | "bool" | "string"
	def  string // default value; empty for no-default keys
	must bool   // false => omit when env unset (no template default)
}

var settings = []setting{
	{"alt-speed-down", "TRANSMISSION__ALT_SPEED_DOWN", "int", "50", true},
	{"alt-speed-enabled", "TRANSMISSION__ALT_SPEED_ENABLED", "bool", "false", true},
	{"alt-speed-time-begin", "TRANSMISSION__ALT_SPEED_TIME_BEGIN", "int", "540", true},
	{"alt-speed-time-day", "TRANSMISSION__ALT_SPEED_TIME_DAY", "int", "127", true},
	{"alt-speed-time-enabled", "TRANSMISSION__ALT_SPEED_TIME_ENABLED", "bool", "false", true},
	{"alt-speed-time-end", "TRANSMISSION__ALT_SPEED_TIME_END", "int", "1020", true},
	{"alt-speed-up", "TRANSMISSION__ALT_SPEED_UP", "int", "50", true},
	{"announce-ip", "TRANSMISSION__ANNOUNCE_IP", "string", "", false},
	{"announce-ip-enabled", "TRANSMISSION__ANNOUNCE_IP_ENABLED", "bool", "false", true},
	{"anti-brute-force-enabled", "TRANSMISSION__ANTI_BRUTE_FORCE_ENABLED", "bool", "false", true},
	{"anti-brute-force-threshold", "TRANSMISSION__ANTI_BRUTE_FORCE_THRESHOLD", "int", "100", true},
	{"bind-address-ipv4", "TRANSMISSION__BIND_ADDRESS_IPV4", "string", "0.0.0.0", true},
	{"bind-address-ipv6", "TRANSMISSION__BIND_ADDRESS_IPV6", "string", "::", true},
	{"blocklist-enabled", "TRANSMISSION__BLOCKLIST_ENABLED", "bool", "false", true},
	{"blocklist-url", "TRANSMISSION__BLOCKLIST_URL", "string", "", false},
	{"cache-size-mb", "TRANSMISSION__CACHE_SIZE_MB", "int", "4", true},
	{"default-trackers", "TRANSMISSION__DEFAULT_TRACKERS", "string", "", false},
	{"dht-enabled", "TRANSMISSION__DHT_ENABLED", "bool", "true", true},
	{"download-dir", "TRANSMISSION__DOWNLOAD_DIR", "string", "/downloads/complete", true},
	{"download-queue-enabled", "TRANSMISSION__DOWNLOAD_QUEUE_ENABLED", "bool", "true", true},
	{"download-queue-size", "TRANSMISSION__DOWNLOAD_QUEUE_SIZE", "int", "5", true},
	{"encryption", "TRANSMISSION__ENCRYPTION", "int", "1", true},
	{"idle-seeding-limit", "TRANSMISSION__IDLE_SEEDING_LIMIT", "int", "30", true},
	{"idle-seeding-limit-enabled", "TRANSMISSION__IDLE_SEEDING_LIMIT_ENABLED", "bool", "false", true},
	{"incomplete-dir", "TRANSMISSION__INCOMPLETE_DIR", "string", "/downloads/incomplete", true},
	{"incomplete-dir-enabled", "TRANSMISSION__INCOMPLETE_DIR_ENABLED", "bool", "true", true},
	{"lpd-enabled", "TRANSMISSION__LPD_ENABLED", "bool", "false", true},
	{"message-level", "TRANSMISSION__MESSAGE_LEVEL", "int", "2", true},
	{"peer-congestion-algorithm", "TRANSMISSION__PEER_CONGESTION_ALGORITHM", "string", "", false},
	{"peer-id-ttl-hours", "TRANSMISSION__PEER_ID_TTL_HOURS", "int", "6", true},
	{"peer-limit-global", "TRANSMISSION__PEER_LIMIT_GLOBAL", "int", "200", true},
	{"peer-limit-per-torrent", "TRANSMISSION__PEER_LIMIT_PER_TORRENT", "int", "50", true},
	{"peer-port", "TRANSMISSION__PEER_PORT", "int", "51413", true},
	{"peer-port-random-high", "TRANSMISSION__PEER_PORT_RANDOM_HIGH", "int", "65535", true},
	{"peer-port-random-low", "TRANSMISSION__PEER_PORT_RANDOM_LOW", "int", "49152", true},
	{"peer-port-random-on-start", "TRANSMISSION__PEER_PORT_RANDOM_ON_START", "bool", "false", true},
	{"peer-socket-tos", "TRANSMISSION__PEER_SOCKET_TOS", "string", "le", true},
	{"pex-enabled", "TRANSMISSION__PEX_ENABLED", "bool", "true", true},
	{"port-forwarding-enabled", "TRANSMISSION__PORT_FORWARDING_ENABLED", "bool", "false", true},
	{"preallocation", "TRANSMISSION__PREALLOCATION", "int", "1", true},
	{"prefetch-enabled", "TRANSMISSION__PREFETCH_ENABLED", "bool", "true", true},
	{"queue-stalled-enabled", "TRANSMISSION__QUEUE_STALLED_ENABLED", "bool", "true", true},
	{"queue-stalled-minutes", "TRANSMISSION__QUEUE_STALLED_MINUTES", "int", "30", true},
	{"ratio-limit", "TRANSMISSION__RATIO_LIMIT", "int", "2", true},
	{"ratio-limit-enabled", "TRANSMISSION__RATIO_LIMIT_ENABLED", "bool", "false", true},
	{"rename-partial-files", "TRANSMISSION__RENAME_PARTIAL_FILES", "bool", "true", true},
	{"rpc-authentication-required", "TRANSMISSION__RPC_AUTHENTICATION_REQUIRED", "bool", "false", true},
	{"rpc-bind-address", "TRANSMISSION__RPC_BIND_ADDRESS", "string", "0.0.0.0", true},
	{"rpc-enabled", "TRANSMISSION__RPC_ENABLED", "bool", "true", true},
	{"rpc-host-whitelist", "TRANSMISSION__RPC_HOST_WHITELIST", "string", "", false},
	{"rpc-host-whitelist-enabled", "TRANSMISSION__RPC_HOST_WHITELIST_ENABLED", "bool", "false", true},
	{"rpc-password", "TRANSMISSION__RPC_PASSWORD", "string", "", false},
	{"rpc-port", "TRANSMISSION__RPC_PORT", "int", "9091", true},
	{"rpc-socket-mode", "TRANSMISSION__RPC_SOCKET_MODE", "string", "0750", true},
	{"rpc-url", "TRANSMISSION__RPC_URL", "string", "/transmission/", true},
	{"rpc-username", "TRANSMISSION__RPC_USERNAME", "string", "", false},
	{"rpc-whitelist", "TRANSMISSION__RPC_WHITELIST", "string", "", false},
	{"rpc-whitelist-enabled", "TRANSMISSION__RPC_WHITELIST_ENABLED", "bool", "false", true},
	{"scrape-paused-torrents-enabled", "TRANSMISSION__SCRAPE_PAUSED_TORRENTS_ENABLED", "bool", "true", true},
	{"script-torrent-added-enabled", "TRANSMISSION__SCRIPT_TORRENT_ADDED_ENABLED", "bool", "false", true},
	{"script-torrent-added-filename", "TRANSMISSION__SCRIPT_TORRENT_ADDED_FILENAME", "string", "", false},
	{"script-torrent-done-enabled", "TRANSMISSION__SCRIPT_TORRENT_DONE_ENABLED", "bool", "false", true},
	{"script-torrent-done-filename", "TRANSMISSION__SCRIPT_TORRENT_DONE_FILENAME", "string", "", false},
	{"script-torrent-done-seeding-enabled", "TRANSMISSION__SCRIPT_TORRENT_DONE_SEEDING_ENABLED", "bool", "false", true},
	{"script-torrent-done-seeding-filename", "TRANSMISSION__SCRIPT_TORRENT_DONE_SEEDING_FILENAME", "string", "", false},
	{"seed-queue-enabled", "TRANSMISSION__SEED_QUEUE_ENABLED", "bool", "false", true},
	{"seed-queue-size", "TRANSMISSION__SEED_QUEUE_SIZE", "int", "10", true},
	{"speed-limit-down", "TRANSMISSION__SPEED_LIMIT_DOWN", "int", "100", true},
	{"speed-limit-down-enabled", "TRANSMISSION__SPEED_LIMIT_DOWN_ENABLED", "bool", "false", true},
	{"speed-limit-up", "TRANSMISSION__SPEED_LIMIT_UP", "int", "100", true},
	{"speed-limit-up-enabled", "TRANSMISSION__SPEED_LIMIT_UP_ENABLED", "bool", "false", true},
	{"start-added-torrents", "TRANSMISSION__START_ADDED_TORRENTS", "bool", "true", true},
	{"tcp-enabled", "TRANSMISSION__TCP_ENABLED", "bool", "true", true},
	{"torrent-added-verify-mode", "TRANSMISSION__TORRENT_ADDED_VERIFY_MODE", "string", "fast", true},
	{"trash-original-torrent-files", "TRANSMISSION__TRASH_ORIGINAL_TORRENT_FILES", "bool", "false", true},
	{"umask", "TRANSMISSION__UMASK", "string", "002", true},
	{"upload-slots-per-torrent", "TRANSMISSION__UPLOAD_SLOTS_PER_TORRENT", "int", "14", true},
	{"utp-enabled", "TRANSMISSION__UTP_ENABLED", "bool", "true", true},
	{"watch-dir", "TRANSMISSION__WATCH_DIR", "string", "/watch", true},
	{"watch-dir-enabled", "TRANSMISSION__WATCH_DIR_ENABLED", "bool", "false", true},
	{"watch-dir-force-generic", "TRANSMISSION__WATCH_FORCE_GENERIC", "bool", "false", true},
}

func fatal(format string, a ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", a...)
	os.Exit(1)
}

func render() string {
	var b strings.Builder
	b.WriteString("{\n")
	first := true
	for _, s := range settings {
		val, ok := os.LookupEnv(s.env)
		if !ok {
			if !s.must {
				continue
			}
			val = s.def
		}
		var enc string
		switch s.typ {
		case "int":
			if _, err := strconv.Atoi(val); err != nil {
				fatal("%s: expected an integer, got %q", s.env, val)
			}
			enc = val
		case "bool":
			if val != "true" && val != "false" {
				fatal("%s: expected true or false, got %q", s.env, val)
			}
			enc = val
		default:
			q, _ := json.Marshal(val)
			enc = string(q)
		}
		if !first {
			b.WriteString(",\n")
		}
		first = false
		fmt.Fprintf(&b, "  %q: %s", s.key, enc)
	}
	b.WriteString("\n}\n")
	return b.String()
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Println("transmission-init " + version)
		return
	}

	anySet := false
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "TRANSMISSION__") {
			anySet = true
			break
		}
	}
	if anySet {
		out := render()
		if !json.Valid([]byte(out)) {
			fatal("internal error: rendered settings.json is not valid JSON")
		}
		if err := os.MkdirAll("/config", 0o755); err != nil {
			fatal("creating /config: %v", err)
		}
		if err := os.WriteFile("/config/settings.json", []byte(out), 0o644); err != nil {
			fatal("writing /config/settings.json: %v", err)
		}
	}

	logLevel := os.Getenv("TRANSMISSION_LOG_LEVEL")
	if logLevel == "" {
		logLevel = "info"
	}
	args := []string{"transmission-daemon", "--foreground", "--config-dir", "/config", "--log-level", logLevel}
	args = append(args, os.Args[1:]...)
	if err := syscall.Exec("/usr/bin/transmission-daemon", args, os.Environ()); err != nil {
		fatal("exec transmission-daemon: %v", err)
	}
}
