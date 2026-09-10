package command

import (
	"context"
	"fmt"
	"strings"

	"zenbot/internal/model"
)

type helpCommand struct{ commandBase }

func (c *helpCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	payload := strings.Join([]string{
		fmtHelp(helpHeader, c.engine.GetPrefix()),
		alignHelp(adminCommands),
		`         \n Moderator commands:\n`,
		alignHelp(moderatorCommands),
		`         \n User commands:\n`,
		alignHelp(userCommands),
		fmtHelp(helpExamples, c.engine.GetPrefix(), c.engine.GetPrefix(), c.engine.GetPrefix(), c.engine.GetPrefix(), c.engine.GetPrefix(), c.engine.GetPrefix()),
	}, "")
	payload = strings.ReplaceAll(payload, "\\\\n", "\\n")
	if _, err := c.engine.SendWhisperMessage(c.message.Name, payload); err != nil {
		return model.FAILED, err
	}
	return model.SUCCESSFUL, nil
}

func fmtHelp(format string, args ...string) string {
	values := make([]any, len(args))
	for i, arg := range args {
		values[i] = arg
	}
	return fmt.Sprintf(format, values...)
}

func alignHelp(output string) string {
	lines := strings.Split(output, `\n`)
	longest := 0
	for _, line := range lines {
		if i := strings.IndexByte(line, '-'); i >= 0 {
			if n := len([]rune(line[:i])); n > longest {
				longest = n
			}
		}
	}
	var b strings.Builder
	for _, line := range lines {
		if i := strings.IndexByte(line, '-'); i >= 0 {
			b.WriteString(line[:i])
			for n := len([]rune(line[:i])); n < longest; n++ {
				b.WriteRune(' ')
			}
			b.WriteString(line[i:])
			b.WriteString(`\n`)
		}
	}
	return b.String()
}

const helpHeader = "All commands can be used through '/whisper'\nPrefix: %s \nCommands:\n"

const adminCommands = " grant,access <trip> <role> - grants a role to a trip\n" +
	" sql <SQL>    - runs SQL against the bot database\n" +
	" mine <room> <start|stop> - controls the trip miner in a room\n" +
	" mem,memory   - shows JVM memory usage\n" +
	" prefix <char>   - changes the live command prefix\n" +
	" msgroom,msgchannel <room> <text> - sends a message to another room\n" +
	" replica,bot <channel> - starts a replica in a room\n" +
	" replicaoff <channel> - stops a running replica\n" +
	" replicastatus,status - shows host and replica status\n" +
	" whiskey <channel> <name> - starts an agent replica with a custom nick\n" +
	" restart,reload  - restarts the host and its replicas\n" +
	" shutdown,exit  - stops the application\n"

const moderatorCommands = " activity <trip>   - shows recent activity patterns for a trip\n" +
	" automove <on|off>  - toggles auto-move between configured rooms\n" +
	" captcha <on|off>  - enables or disables captcha\n" +
	" auth,authorize <trip> - authorizes a trip on the room\n" +
	" deauth <trip>   - removes trip authorization\n" +
	" kick,k,out <nick>  - kicks a user from the room\n" +
	" nuke <room>   - locks a room and clears users from it\n" +
	" messages,lastmessages <trip> <count> - shows recent messages for a trip\n" +
	" lock,lockroom <on|off> - locks or unlocks the current room\n" +
	" overflow,shoot <nick> - sends the selected overflow action\n" +
	" register,reg <nick> <trip> - registers or updates a nick/trip pair\n" +
	" remove <name|trip>  - removes a registered user\n" +
	" move <name> <from> <to> - moves a user between rooms\n" +
	" resurrect    - moves the last kicked user back\n" +
	" shadowban,sban <target> - shadow-bans by nick, trip, or hash\n" +
	" shadowbanlist,banlist - lists shadow-banned users\n" +
	" unshadowban <target> - removes a shadow ban\n" +
	" ban <nick>    - bans a user\n" +
	" unban <hash>   - unbans by hash\n" +
	" unbanall    - clears all room bans\n" +
	" mute,dumb <nick>  - mutes a user\n" +
	" unmute <hash>  - unmutes by hash\n" +
	" color <name> <color> - applies a color to an online user\n" +
	" flair <name> <flair> - applies a flair to an online user\n"

const userCommands = " help,h       - shows this help output\n" +
	" afk [reason]   - marks you as AFK\n" +
	" ape,harambe   - prints an ape\n" +
	" howto,hcguide  - shows the moderation crash course\n" +
	" info,whois <nick>  - shows a user's trip and hash\n" +
	" lastseen <name>  - shows when a user was last active\n" +
	" list <channel>   - lists users in a room\n" +
	" msg,mail <nick> <text> - sends mail to a registered user\n" +
	" msgroom <room> <text> - sends a message to another room\n" +
	" nicks,t2n <trip>  - lists known nicks for a trip\n" +
	" notes       - lists your saved notes\n" +
	" note,save <text>  - saves a note\n" +
	" notes purge   - removes all saved notes\n" +
	" ping,p       - shows bot latency\n" +
	" l <prompt>  - asks the room agent a question\n" +
	" users       - lists registered users\n" +
	" say,echo <text>  - echoes text back\n" +
	" sub,subscribe   - subscribes to join notifications\n" +
	" time,t <city|country> - shows local time\n" +
	" unsub,unsubscribe - cancels join notifications\n" +
	" weather,w <city>  - shows weather data\n" +
	" version,v     - shows the running version\n" +
	" ws,wsay <text>  - forwards text to the support relay\n" +
	" wsa <text>    - sends anonymous support relay text\n" +
	" dbzhelp,dbz   - shows DBZ game commands\n"

const helpExamples = "Examples:\n" +
	"  %scaptcha on \n" +
	"  %safk domestic business \n" +
	"  %slist programming \n" +
	"  %sweather nc, charlotte \n" +
	"  %smail santa Get me a native java compiler \n" +
	"  %smsg wwandrew you, tonight \n" +
	"         \n" +
	"  Developed by mercury, _https://github.com/ArteomBalanuta/saturn_\n"
