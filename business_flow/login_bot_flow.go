package businessflow

import (
	"context"

	"github.com/amirphl/Yamata-no-Orochi/app/dto"
	"github.com/amirphl/Yamata-no-Orochi/app/services"
	"github.com/amirphl/Yamata-no-Orochi/repository"
	"github.com/amirphl/Yamata-no-Orochi/utils"
	"golang.org/x/crypto/bcrypt"
)

// BotAuthFlow represents the bot authentication flow used by handlers
type BotAuthFlow interface {
	Verify(ctx context.Context, req *dto.BotLoginRequest, metadata *ClientMetadata) (*dto.BotLoginResponse, error)
}

type BotAuthFlowImpl struct {
	botRepo      repository.BotRepository
	tokenService services.TokenService
}

func NewBotAuthFlow(botRepo repository.BotRepository, tokenService services.TokenService) BotAuthFlow {
	return &BotAuthFlowImpl{
		botRepo:      botRepo,
		tokenService: tokenService,
	}
}

func (bf *BotAuthFlowImpl) Verify(ctx context.Context, req *dto.BotLoginRequest, metadata *ClientMetadata) (*dto.BotLoginResponse, error) {
	if req == nil || len(req.Username) == 0 || len(req.Password) == 0 {
		return nil, NewBusinessError("BOT_LOGIN_VALIDATION_FAILED", "Bot login validation failed", ErrIncorrectPassword)
	}

	bot, err := bf.botRepo.ByUsername(ctx, req.Username)
	if err != nil {
		return nil, NewBusinessError("BOT_LOOKUP_FAILED", "Failed to lookup bot", err)
	}
	if bot == nil {
		return nil, NewBusinessError("BOT_NOT_FOUND", "Bot not found", ErrBotNotFound)
	}
	if !utils.IsTrue(bot.IsActive) {
		return nil, NewBusinessError("BOT_INACTIVE", "Bot account is inactive", ErrBotInactive)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(bot.PasswordHash), []byte(req.Password)); err != nil {
		return nil, NewBusinessError("BOT_INCORRECT_PASSWORD", "Incorrect password", ErrIncorrectPassword)
	}

	accessToken, refreshToken, err := bf.tokenService.GenerateBotTokens(bot.ID)
	if err != nil {
		return nil, NewBusinessError("TOKEN_GENERATION_FAILED", "Failed to generate tokens", err)
	}

	resp := &dto.BotLoginResponse{
		Bot:     ToBotDTOModel(*bot),
		Session: ToBotSessionDTO(accessToken, refreshToken),
	}
	return resp, nil
}
