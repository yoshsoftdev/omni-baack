package v1

import (
	"github.com/omnichannel/omni-chan-v2/internal/api/v1/handler"
	"github.com/omnichannel/omni-chan-v2/internal/auth"
	jwtmanager "github.com/omnichannel/omni-chan-v2/pkg/jwt"
)

// Deps wires HTTP v1 handlers and shared middleware dependencies.
type Deps struct {
	Auth         *handler.AuthHandler
	Workspace    *handler.WorkspaceHandler
	Team         *handler.TeamHandler
	Channel      *handler.ChannelHandler
	Template     *handler.TemplateHandler
	Contact      *handler.ContactHandler
	Segment      *handler.SegmentHandler
	Tag          *handler.TagHandler
	Campaign     *handler.CampaignHandler
	Report       *handler.ReportHandler
	Conversation *handler.ConversationHandler
	Message      *handler.MessageHandler
	Dashboard    *handler.DashboardHandler
	ActorResolver auth.ActorResolver
	JWT           *jwtmanager.Manager
}
