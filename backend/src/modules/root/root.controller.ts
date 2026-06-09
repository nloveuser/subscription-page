import { Request, Response } from 'express';

import { Get, Post, Controller, Res, Req, Param, Logger, HttpCode } from '@nestjs/common';
import { ConfigService } from '@nestjs/config';

import {
    REQUEST_TEMPLATE_TYPE_VALUES,
    TRequestTemplateTypeKeys,
} from '@remnawave/backend-contract';
import { APP_CONFIG_ROUTE_WO_LEADING_PATH } from '@remnawave/subscription-page-types';

import { GetJWTPayload } from '@common/decorators/get-jwt-payload';
import { ClientIp } from '@common/decorators/get-ip';
import { IJwtPayload } from '@common/constants';

import { SubpageConfigService } from './subpage-config.service';
import { RootService } from './root.service';
// DomainsConfigService reload is triggered inside SubpageConfigService.reloadConfigs()

@Controller()
export class RootController {
    private readonly logger = new Logger(RootController.name);

    constructor(
        private readonly rootService: RootService,
        private readonly subpageConfigService: SubpageConfigService,
        private readonly configService: ConfigService,
    ) {}

    @Get('')
    async homepage(@Req() request: Request, @Res() response: Response) {
        return await this.rootService.serveHomepage(request, response);
    }

    @Get(APP_CONFIG_ROUTE_WO_LEADING_PATH)
    async getSubscriptionPageConfig(@GetJWTPayload() user: IJwtPayload, @Req() request: Request) {
        return await this.subpageConfigService.getSubscriptionPageConfig(user.su, request);
    }

    @Post('internal/reload')
    @HttpCode(200)
    async reloadConfig(@Req() request: Request, @Res() response: Response) {
        const expectedToken = this.configService.get<string>('INTERNAL_API_TOKEN');
        const providedToken = request.headers['x-internal-token'];

        if (expectedToken && providedToken !== expectedToken) {
            response.status(401).json({ error: 'Unauthorized' });
            return;
        }

        const result = await this.subpageConfigService.reloadConfigs();

        if (!result.success) {
            response.status(500).json({ error: result.error });
            return;
        }

        response.status(200).json({ status: 'ok', message: 'Configs reloaded successfully' });
    }

    @Get([':shortUuid', ':shortUuid/:clientType'])
    async root(
        @ClientIp() clientIp: string,
        @Req() request: Request,
        @Res() response: Response,
        @Param('shortUuid') shortUuid: string,
        @Param('clientType') clientType: string,
    ) {
        if (request.path.startsWith('/assets') || request.path.startsWith('/locales')) {
            response.socket?.destroy();
            return;
        }

        if (clientType === undefined) {
            return await this.rootService.serveSubscriptionPage(
                clientIp,
                request,
                response,
                shortUuid,
            );
        }

        if (!REQUEST_TEMPLATE_TYPE_VALUES.includes(clientType as TRequestTemplateTypeKeys)) {
            this.logger.error(`Invalid client type: ${clientType}`);

            response.socket?.destroy();
            return;
        } else {
            return await this.rootService.serveSubscriptionPage(
                clientIp,
                request,
                response,
                shortUuid,
                clientType as TRequestTemplateTypeKeys,
            );
        }
    }
}
