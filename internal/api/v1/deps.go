package v1

import (
	"github.com/omnichannel/omni-chan-v2/internal/api/v1/handler"
	"github.com/omnichannel/omni-chan-v2/internal/auth"
	metasvc "github.com/omnichannel/omni-chan-v2/internal/meta"
	jwtmanager "github.com/omnichannel/omni-chan-v2/pkg/jwt"
)

// Deps wires HTTP v1 handlers and shared middleware dependencies.
type Deps struct {
	Auth                 *handler.AuthHandler
	Workspace            *handler.WorkspaceHandler
	Team                 *handler.TeamHandler
	Channel              *handler.ChannelHandler
	ChannelManagement    *handler.ChannelManagementHandler
	InstagramOAuth       *handler.InstagramOAuthHandler
	Template             *handler.TemplateHandler
	Contact              *handler.ContactHandler
	Provider             *handler.ProviderHandler
	Segment              *handler.SegmentHandler
	Tag                  *handler.TagHandler
	QuickReply           *handler.QuickReplyHandler
	Automation           *handler.AutomationHandler
	Campaign             *handler.CampaignHandler
	Operations           *handler.OperationsHandler
	Report               *handler.ReportHandler
	Conversation         *handler.ConversationHandler
	ConversationPresence *handler.ConversationPresenceHandler
	Inbox                *handler.InboxHandler
	Message              *handler.MessageHandler
	MediaProxy           *handler.MediaProxyHandler
	StorageServiceURL    string
	Dashboard            *handler.DashboardHandler
	Webhook              *handler.WebhookHandler
	Meta                 *metasvc.Handler
	ActorResolver        auth.ActorResolver
	JWT                  *jwtmanager.Manager
}
