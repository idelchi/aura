package cli

import (
	cli "github.com/urfave/cli/v3"

	"github.com/idelchi/aura/internal/cli/cachecmd"
	"github.com/idelchi/aura/internal/cli/core"
	initialize "github.com/idelchi/aura/internal/cli/init"
	"github.com/idelchi/aura/internal/cli/login"
	mcpcmd "github.com/idelchi/aura/internal/cli/mcp"
	"github.com/idelchi/aura/internal/cli/models"
	"github.com/idelchi/aura/internal/cli/plugins"
	"github.com/idelchi/aura/internal/cli/query"
	"github.com/idelchi/aura/internal/cli/run"
	"github.com/idelchi/aura/internal/cli/show"
	"github.com/idelchi/aura/internal/cli/skills"
	"github.com/idelchi/aura/internal/cli/speak"
	"github.com/idelchi/aura/internal/cli/tasks"
	"github.com/idelchi/aura/internal/cli/tokens"
	"github.com/idelchi/aura/internal/cli/tools"
	"github.com/idelchi/aura/internal/cli/transcribe"
	"github.com/idelchi/aura/internal/cli/vision"
	"github.com/idelchi/aura/internal/cli/web"
)

func buildSubcommands(flags *core.Flags) []*cli.Command {
	return []*cli.Command{
		models.Command(flags),
		tools.Command(flags),
		run.Command(flags),
		initialize.Command(flags),
		query.Command(flags),
		vision.Command(),
		transcribe.Command(flags),
		speak.Command(flags),
		tasks.Command(flags),
		plugins.Command(flags),
		skills.Command(flags),
		login.Command(flags),
		mcpcmd.Command(),
		web.Command(flags),
		tokens.Command(flags),
		cachecmd.Command(flags),
		show.Command(flags),
	}
}
