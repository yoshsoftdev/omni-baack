package v1

import (
	"github.com/gofiber/fiber/v2"
	"github.com/omnichannel/omni-chan-v2/internal/api/middleware"
	"github.com/omnichannel/omni-chan-v2/internal/api/response"
	"github.com/omnichannel/omni-chan-v2/internal/storageproxy"
)

func Register(router fiber.Router, deps Deps) {
	router.Get("/health", health)

	// Internal-only media stream consumed by webhook-service.
	if deps.MediaProxy != nil {
		router.Get("/internal/media/:token", deps.MediaProxy.Serve)
	}

	if deps.Webhook != nil {
		router.Get("/webhook/meta", deps.Webhook.VerifyMeta)
	}

	if deps.InstagramOAuth != nil {
		router.Get("/channels/instagram/callback", deps.InstagramOAuth.Callback)
	}

	if deps.JWT == nil {
		return
	}

	authGroup := router.Group("/auth")
	if deps.Auth != nil {
		authGroup.Post("/register", deps.Auth.Register)
		authGroup.Post("/login", deps.Auth.Login)
		authGroup.Post("/refresh", deps.Auth.Refresh)
		authGroup.Post("/logout", deps.Auth.Logout)
		authGroup.Get("/me", middleware.Auth(deps.JWT), middleware.ResolveActor(deps.ActorResolver), deps.Auth.Me)
		router.Get("/invitations/:id", deps.Auth.GetInvitation)
		router.Post("/invitations/:id/accept", deps.Auth.AcceptInvitation)
	}

	protected := router.Group("", middleware.Auth(deps.JWT), middleware.ResolveActor(deps.ActorResolver))

	if deps.Workspace != nil {
		workspace := protected.Group("/workspace")
		workspace.Get("/", deps.Workspace.Get)
		workspace.Patch("/", deps.Workspace.Update)
		workspace.Get("/quotas", deps.Workspace.GetQuotas)
		workspace.Get("/readiness", deps.Workspace.GetReadiness)
		workspace.Post("/readiness/refresh", deps.Workspace.RefreshReadiness)
	}

	if deps.Team != nil {
		teams := protected.Group("/teams")
		teams.Get("/members", deps.Team.ListMembers)
		teams.Post("/members", deps.Team.AddMember)
		teams.Get("/users", deps.Team.ListUsers)
		teams.Post("/users/invite", deps.Team.InviteUser)
		teams.Patch("/users/:id/role", deps.Team.UpdateUserRole)
		teams.Post("/users/:id/deactivate", deps.Team.DeactivateUser)
		teams.Delete("/users/:id", deps.Team.RemoveUser)
		teams.Get("/roles", deps.Team.ListRoles)
		teams.Post("/roles", deps.Team.CreateRole)
		teams.Get("/roles/:id", deps.Team.GetRole)
		teams.Patch("/roles/:id", deps.Team.UpdateRole)
		teams.Delete("/roles/:id", deps.Team.DeleteRole)
		teams.Put("/roles/:id/permissions", deps.Team.UpdateRolePermissions)
		teams.Get("/permissions", deps.Team.GetPermissions)
	}

	if deps.Channel != nil {
		channels := protected.Group("/channels")
		channels.Get("/", deps.Channel.List)
		channels.Post("/manual/verify", deps.Channel.VerifyManualConnect)
		channels.Post("/", deps.Channel.Create)
		if deps.ChannelManagement != nil {
			channels.Get("/types", deps.ChannelManagement.ListTypes)
			channels.Get("/status", deps.ChannelManagement.Status)
		}
		if deps.InstagramOAuth != nil {
			channels.Get("/instagram/connect-url", deps.InstagramOAuth.ConnectURL)
			channels.Get("/instagram/status", deps.InstagramOAuth.Status)
			channels.Delete("/instagram/connection", deps.InstagramOAuth.Disconnect)
		}
		channels.Get("/:id/health", deps.Channel.Health)
		channels.Get("/:id/payment-status", deps.Channel.GetPaymentStatus)
		channels.Post("/:id/payment-link", deps.Channel.CreatePaymentLink)
		channels.Post("/:id/embedded-signup/start", deps.Channel.StartEmbeddedSignup)
		channels.Get("/:id/embedded-signup/:session_id", deps.Channel.GetEmbeddedSignupStatus)
		channels.Post("/:id/embedded-signup/:session_id/complete", deps.Channel.CompleteEmbeddedSignup)
		channels.Get("/:id/sync/status", deps.Channel.GetSyncStatus)
		channels.Post("/:id/sync/contacts", deps.Channel.SyncContacts)
		channels.Post("/:id/sync/conversations", deps.Channel.SyncConversations)
		channels.Get("/:id/permissions", deps.Channel.GetPermissions)
		channels.Post("/:id/permissions/reauthorize", deps.Channel.ReauthorizePermissions)
		channels.Get("/:id", deps.Channel.Get)
		channels.Patch("/:id", deps.Channel.Update)
		channels.Delete("/:id", deps.Channel.Delete)

		embeddedSignup := protected.Group("/embedded-signup")
		embeddedSignup.Post("/start", deps.Channel.StartTeamEmbeddedSignup)
		embeddedSignup.Get("/:session_id", deps.Channel.GetTeamEmbeddedSignupStatus)
		embeddedSignup.Post("/:session_id/complete", deps.Channel.CompleteTeamEmbeddedSignup)
	} else if deps.ChannelManagement != nil || deps.InstagramOAuth != nil {
		channels := protected.Group("/channels")
		if deps.ChannelManagement != nil {
			channels.Get("/types", deps.ChannelManagement.ListTypes)
			channels.Get("/status", deps.ChannelManagement.Status)
		}
		if deps.InstagramOAuth != nil {
			channels.Get("/instagram/connect-url", deps.InstagramOAuth.ConnectURL)
			channels.Get("/instagram/status", deps.InstagramOAuth.Status)
			channels.Delete("/instagram/connection", deps.InstagramOAuth.Disconnect)
		}
	}

	if deps.Template != nil {
		templates := protected.Group("/templates")
		templates.Get("/", deps.Template.List)
		templates.Post("/", deps.Template.Create)
		templates.Get("/sync/history", deps.Template.SyncHistory)
		templates.Get("/sync/history/:id", deps.Template.GetSyncHistory)
		templates.Post("/sync", deps.Template.Sync)
		templates.Get("/:id", deps.Template.Get)
		templates.Delete("/:id", deps.Template.Delete)
	}

	if deps.Contact != nil {
		contacts := protected.Group("/contacts")
		contacts.Get("/stats", deps.Contact.Stats)
		contacts.Get("/field-definitions", deps.Contact.ListFieldDefinitions)
		contacts.Post("/field-definitions", deps.Contact.CreateFieldDefinition)
		contacts.Patch("/field-definitions/:id", deps.Contact.UpdateFieldDefinition)
		contacts.Delete("/field-definitions/:id", deps.Contact.DeleteFieldDefinition)
		contacts.Get("/", deps.Contact.List)
		contacts.Post("/", deps.Contact.Create)
		if deps.Provider != nil {
			contacts.Get("/:id/providers", deps.Provider.ListByContact)
			contacts.Get("/:id/activities", deps.Provider.ListActivities)
		}
		contacts.Get("/:id/links", deps.Contact.ListLinks)
		contacts.Post("/:id/links", deps.Contact.LinkContact)
		contacts.Delete("/:id/links/:linkedContactId", deps.Contact.UnlinkContact)
		contacts.Get("/:id", deps.Contact.Get)
		contacts.Patch("/:id", deps.Contact.Update)
		contacts.Delete("/:id", deps.Contact.Delete)
	}

	if deps.Provider != nil {
		providers := protected.Group("/providers")
		providers.Get("/", deps.Provider.List)
		providers.Get("/:id", deps.Provider.Get)
		providers.Post("/:id/link", deps.Provider.Link)
		providers.Post("/:id/unlink", deps.Provider.Unlink)
		providers.Patch("/:id/source-of-truth", deps.Provider.SetSourceOfTruth)
	}

	if deps.Segment != nil {
		segments := protected.Group("/segments")
		segments.Get("/", deps.Segment.List)
		segments.Post("/", deps.Segment.Create)
		segments.Get("/:id", deps.Segment.Get)
		segments.Patch("/:id", deps.Segment.Update)
		segments.Delete("/:id", deps.Segment.Delete)
	}

	if deps.Tag != nil {
		tags := protected.Group("/tags")
		tags.Get("/", deps.Tag.List)
		tags.Post("/", deps.Tag.Create)
		tags.Get("/:id", deps.Tag.Get)
		tags.Patch("/:id", deps.Tag.Update)
		tags.Delete("/:id", deps.Tag.Delete)
		tags.Post("/:id/contacts/:contact_id", deps.Tag.Assign)
		tags.Delete("/:id/contacts/:contact_id", deps.Tag.Unassign)
	}

	if deps.QuickReply != nil {
		quickReplies := protected.Group("/quick-replies")
		quickReplies.Get("/", deps.QuickReply.List)
		quickReplies.Post("/", deps.QuickReply.Create)
		quickReplies.Get("/:id", deps.QuickReply.Get)
		quickReplies.Patch("/:id", deps.QuickReply.Update)
		quickReplies.Delete("/:id", deps.QuickReply.Delete)
	}

	if deps.Campaign != nil {
		campaigns := protected.Group("/campaigns")
		campaigns.Get("/", deps.Campaign.List)
		campaigns.Post("/", deps.Campaign.Create)
		campaigns.Get("/:id", deps.Campaign.Get)
		campaigns.Patch("/:id", deps.Campaign.Update)
		campaigns.Delete("/:id", deps.Campaign.Delete)
		campaigns.Post("/:id/send", deps.Campaign.Send)
		campaigns.Post("/:id/cancel", deps.Campaign.Cancel)
	}

	if deps.Operations != nil {
		ops := protected.Group("/operations")
		ops.Get("/broadcast", deps.Operations.BroadcastStatus)
		ops.Get("/workers", deps.Operations.WorkerStatus)
	}

	if deps.Report != nil {
		reports := protected.Group("/reports")
		reports.Get("/dashboard", deps.Report.Dashboard)
		reports.Get("/daily", deps.Report.DailyTrend)
		reports.Get("/campaigns", deps.Report.ListCampaigns)
		reports.Get("/campaigns/:id", deps.Report.GetCampaign)
		reports.Get("/campaigns/:id/clicks", deps.Report.ListLinkClicks)
	}

	if deps.Conversation != nil {
		conversations := protected.Group("/conversations")
		conversations.Get("/", deps.Conversation.List)
		conversations.Post("/", deps.Conversation.Create)
		conversations.Get("/:id", deps.Conversation.Get)
		conversations.Post("/:id/assign", deps.Conversation.Assign)
		conversations.Patch("/:id/status", deps.Conversation.UpdateStatus)
		conversations.Post("/:id/read", deps.Conversation.MarkRead)
		conversations.Get("/:id/activities", deps.Conversation.ListActivities)
		conversations.Get("/:id/notes", deps.Conversation.ListNotes)
		conversations.Post("/:id/notes", deps.Conversation.CreateNote)
		conversations.Delete("/:id/notes/:noteId", deps.Conversation.DeleteNote)
		if deps.ConversationPresence != nil {
			conversations.Post("/:id/presence/open", deps.ConversationPresence.Open)
			conversations.Post("/:id/presence/close", deps.ConversationPresence.Close)
			conversations.Post("/:id/presence/typing", deps.ConversationPresence.Typing)
			conversations.Get("/:id/presence", deps.ConversationPresence.Get)
		}
		if deps.Message != nil {
			conversations.Get("/:id/messages", deps.Message.List)
			conversations.Post("/:id/messages", deps.Message.Send)
			conversations.Delete("/:id/messages/:messageId", deps.Message.Delete)
		}
	}

	if deps.Inbox != nil {
		protected.Get("/inbox", deps.Inbox.List)
	}

	if deps.Workspace != nil {
		onboarding := protected.Group("/onboarding")
		onboarding.Get("/", deps.Workspace.GetOnboarding)
		onboarding.Post("/complete-step", deps.Workspace.CompleteOnboardingStep)
	}

	if deps.StorageServiceURL != "" {
		storageProxy := storageproxy.Handler(deps.StorageServiceURL)
		storageGroup := protected.Group("/storage")
		storageGroup.Post("/upload", storageProxy)
		storageGroup.Get("/blob/*", storageProxy)
		storageGroup.Get("/objects/:id/content", storageProxy)
		storageGroup.Get("/objects/:id", storageProxy)
		storageGroup.Delete("/objects/:id", storageProxy)
	}

	if deps.Dashboard != nil {
		protected.Get("/dashboard", deps.Dashboard.Get)
	}

	if deps.Meta != nil {
		meta := protected.Group("/meta")
		meta.Get("/embedded-config", deps.Meta.EmbeddedConfig)
	}
}

func health(c *fiber.Ctx) error {
	return response.OK(c, "ok", fiber.Map{
		"version": "v1",
	})
}
