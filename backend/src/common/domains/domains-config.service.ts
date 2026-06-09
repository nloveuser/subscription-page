import { Injectable, Logger, OnApplicationBootstrap } from '@nestjs/common';
import { ConfigService } from '@nestjs/config';
import axios from 'axios';

import { IDomainConfig } from './domain-config.interface';

@Injectable()
export class DomainsConfigService implements OnApplicationBootstrap {
    private readonly logger = new Logger(DomainsConfigService.name);
    private domainMap: Map<string, IDomainConfig> = new Map();

    constructor(private readonly configService: ConfigService) {}

    async onApplicationBootstrap(): Promise<void> {
        await this.reload();
    }

    async reload(): Promise<void> {
        const apiUrl = this.configService.get<string>('DOMAINS_API_URL');
        if (!apiUrl) {
            this.logger.log('DOMAINS_API_URL not set — multi-domain support disabled.');
            return;
        }

        const token = this.configService.get<string>('INTERNAL_API_TOKEN');

        try {
            const response = await axios.get<IDomainConfig[]>(`${apiUrl}/api/domains`, {
                headers: token ? { 'X-Internal-Token': token } : {},
                timeout: 10_000,
            });

            const configs: IDomainConfig[] = Array.isArray(response.data) ? response.data : [];
            const newMap = new Map<string, IDomainConfig>();

            for (const cfg of configs) {
                if (cfg.domain) {
                    newMap.set(cfg.domain.toLowerCase(), cfg);
                }
            }

            this.domainMap = newMap;
            this.logger.log(`Loaded ${this.domainMap.size} domain config(s) from Go API.`);
        } catch (err: unknown) {
            const msg = err instanceof Error ? err.message : String(err);
            this.logger.warn(`Failed to load domain configs from Go API: ${msg}`);
            // Non-fatal: system works in single-domain mode
        }
    }

    getForHost(host: string): IDomainConfig | undefined {
        const hostname = host.split(':')[0].toLowerCase();
        return this.domainMap.get(hostname);
    }

    get size(): number {
        return this.domainMap.size;
    }
}
