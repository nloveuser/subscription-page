import { exit } from 'node:process';
import { Request } from 'express';

import { Injectable, OnApplicationBootstrap } from '@nestjs/common';
import { ConfigService } from '@nestjs/config';
import { Logger } from '@nestjs/common';

import {
    SubscriptionPageRawConfigSchema,
    TSubscriptionPageRawConfig,
    SUBPAGE_DEFAULT_CONFIG_UUID,
} from '@remnawave/subscription-page-types';

import { decryptUuid, encryptUuid } from '@common/utils/crypt-utils';
import { AxiosService } from '@common/axios';
import { IDomainConfig } from '@common/domains/domain-config.interface';
import { DomainsConfigService } from '@common/domains/domains-config.service';

@Injectable()
export class SubpageConfigService implements OnApplicationBootstrap {
    private readonly logger = new Logger(SubpageConfigService.name);
    private readonly internalJwtSecret: string;
    private readonly subpageConfigUuid: string;
    private readonly subpageConfigMap: Map<string, TSubscriptionPageRawConfig> = new Map();
    private readonly nameToUuidMap: Map<string, string> = new Map();

    constructor(
        private readonly configService: ConfigService,
        private readonly axiosService: AxiosService,
        private readonly domainsConfigService: DomainsConfigService,
    ) {
        this.internalJwtSecret = this.configService.getOrThrow<string>('INTERNAL_JWT_SECRET');
        this.subpageConfigUuid = this.configService.getOrThrow<string>('SUBPAGE_CONFIG_UUID');
    }

    public async onApplicationBootstrap(): Promise<void> {
        const ok = await this.loadConfigsInto(
            this.subpageConfigMap,
            this.nameToUuidMap,
            { exitOnFailure: true },
        );
        if (!ok) exit(1);
        this.logger.log('[OK] Subpage configs are loaded successfully.');
    }

    public async reloadConfigs(): Promise<{ success: boolean; error?: string }> {
        this.logger.log('Reloading subpage configs...');
        const newMap = new Map<string, TSubscriptionPageRawConfig>();
        const newNameMap = new Map<string, string>();

        const ok = await this.loadConfigsInto(newMap, newNameMap, { exitOnFailure: false });
        if (!ok) {
            return { success: false, error: 'Failed to reload configs from panel' };
        }

        this.subpageConfigMap.clear();
        for (const [k, v] of newMap) this.subpageConfigMap.set(k, v);

        this.nameToUuidMap.clear();
        for (const [k, v] of newNameMap) this.nameToUuidMap.set(k, v);

        // Also reload domain configs from Go API
        await this.domainsConfigService.reload();

        this.logger.log(`Reloaded ${this.subpageConfigMap.size} subpage config(s), ${this.nameToUuidMap.size} name(s).`);
        return { success: true };
    }

    public async getSubscriptionPageConfig(
        encryptedSubpageConfigUuid: string,
        req: Request,
    ): Promise<object | void> {
        const decryptedSubpageConfigUuid = decryptUuid(
            encryptedSubpageConfigUuid,
            this.internalJwtSecret,
        );

        if (!decryptedSubpageConfigUuid) {
            req.socket?.destroy();
            return;
        }

        const subpageConfig = this.subpageConfigMap.get(decryptedSubpageConfigUuid);

        if (!subpageConfig) {
            this.logger.error(`[FATAL] SubPage config ${decryptedSubpageConfigUuid} not found`);
            req.socket?.destroy();
            return;
        }

        // Apply per-domain branding overrides (from Go API domain config or global env)
        const host = (req.headers['host'] as string | undefined)?.split(':')[0] ?? '';
        const domainConfig = this.domainsConfigService.getForHost(host);

        return this.applyBrandingOverrides(subpageConfig, domainConfig);
    }

    public getEncryptedSubpageConfigUuid(subpageConfigUuidFromRemnawave: string | null): string {
        return encryptUuid(
            this.getFinalSubpageConfigUuid(subpageConfigUuidFromRemnawave),
            this.internalJwtSecret,
        );
    }

    /** Resolve config UUID by panel config name (set in domains.json). */
    public getConfigUuidByName(name: string): string | undefined {
        return this.nameToUuidMap.get(name);
    }

    public getBaseSettings(
        subpageConfigUuid: string | null,
        domainConfig?: IDomainConfig,
    ): TSubscriptionPageRawConfig['baseSettings'] {
        const subpageConfig = this.subpageConfigMap.get(
            this.getFinalSubpageConfigUuid(subpageConfigUuid),
        );

        const metaTitle =
            domainConfig?.meta?.title ||
            this.configService.get<string>('META_TITLE') ||
            subpageConfig?.baseSettings?.metaTitle ||
            'Subscription Page';

        const metaDescription =
            domainConfig?.meta?.description ||
            this.configService.get<string>('META_DESCRIPTION') ||
            subpageConfig?.baseSettings?.metaDescription ||
            'Subscription Page';

        const showConnectionKeys =
            domainConfig?.ui?.showConnectionKeys ??
            subpageConfig?.baseSettings?.showConnectionKeys ??
            false;

        const hideGetLinkButton =
            domainConfig?.ui?.hideGetLinkButton ??
            subpageConfig?.baseSettings?.hideGetLinkButton ??
            false;

        return { metaTitle, metaDescription, showConnectionKeys, hideGetLinkButton };
    }

    private applyBrandingOverrides(
        config: TSubscriptionPageRawConfig,
        domainConfig?: IDomainConfig,
    ): TSubscriptionPageRawConfig {
        const brandName =
            domainConfig?.brand?.name || this.configService.get<string>('BRAND_NAME');
        const brandSupportUrl =
            domainConfig?.brand?.supportUrl || this.configService.get<string>('BRAND_SUPPORT_URL');
        const brandLogoUrl =
            domainConfig?.brand?.logoUrl || this.configService.get<string>('BRAND_LOGO_URL');

        if (!brandName && !brandSupportUrl && !brandLogoUrl) return config;

        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        const cloned = structuredClone(config) as any;
        if (brandName) cloned.brandingSettings.title = brandName;
        if (brandSupportUrl) cloned.brandingSettings.supportUrl = brandSupportUrl;
        if (brandLogoUrl) cloned.brandingSettings.logoUrl = brandLogoUrl;
        return cloned as TSubscriptionPageRawConfig;
    }

    private async loadConfigsInto(
        target: Map<string, TSubscriptionPageRawConfig>,
        nameTarget: Map<string, string>,
        opts: { exitOnFailure: boolean },
    ): Promise<boolean> {
        const configEntries = await this.fetchSubscriptionPageConfigList();

        if (configEntries.length === 0) {
            this.logger.error('[FATAL] Subscription page config list is empty.');
            return false;
        }

        this.logger.log(`Found ${configEntries.length} subscription page config(s).`);

        for (const entry of configEntries) {
            const subscriptionPageConfig =
                await this.axiosService.getSubscriptionPageConfigByUuid(entry.uuid);

            if (!subscriptionPageConfig.isOk || !subscriptionPageConfig.response) {
                this.logger.error(
                    `[FATAL] Error while fetching subpage config: ${entry.uuid}`,
                );
                if (opts.exitOnFailure) exit(1);
                return false;
            }

            const parsedConfig = await SubscriptionPageRawConfigSchema.safeParseAsync(
                subscriptionPageConfig.response.config,
            );

            if (!parsedConfig.success) {
                this.logger.error(
                    `[FATAL] ${entry.uuid} is not valid: ${JSON.stringify(parsedConfig.error)}`,
                );
                if (opts.exitOnFailure) exit(1);
                return false;
            }

            this.logger.log(`[OK] ${entry.uuid} (name: ${entry.name})`);
            target.set(entry.uuid, parsedConfig.data);
            if (entry.name) {
                nameTarget.set(entry.name, entry.uuid);
            }
        }

        if (target.size === 0) {
            this.logger.error('[FAILED] At least one SubPage config must be valid!');
            return false;
        }

        return true;
    }

    private async fetchSubscriptionPageConfigList(): Promise<Array<{ uuid: string; name: string }>> {
        const subscriptionPageConfigList = await this.axiosService.getSubscriptionPageConfigList();
        if (!subscriptionPageConfigList.isOk || !subscriptionPageConfigList.response) {
            this.logger.error('Subscription page config list cannot be fetched');
            return [];
        }

        return subscriptionPageConfigList.response.configs.map((config) => ({
            uuid: config.uuid,
            // eslint-disable-next-line @typescript-eslint/no-explicit-any
            name: (config as any).name ?? '',
        }));
    }

    private getFinalSubpageConfigUuid(subpageConfigUuid: string | null): string {
        const isDefaultUuid = this.subpageConfigUuid === SUBPAGE_DEFAULT_CONFIG_UUID;

        if (isDefaultUuid && subpageConfigUuid) {
            return subpageConfigUuid;
        }

        return this.subpageConfigUuid;
    }
}
