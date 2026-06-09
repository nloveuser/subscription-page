export interface IDomainBrand {
    name?: string;
    supportUrl?: string;
    logoUrl?: string;
}

export interface IDomainMeta {
    title?: string;
    description?: string;
}

export interface IDomainTheme {
    primaryColor?: string;
    bgColor?: string;
    accentLeftColor?: string;
    accentRightColor?: string;
}

export interface IDomainUI {
    subscriptionInfoBlockType?: string;
    installationGuidesBlockType?: string;
    hideGetLinkButton?: boolean;
    showConnectionKeys?: boolean;
}

export interface IDomainLocale {
    primary?: string;
    additional?: string[];
}

export interface IDomainConfig {
    id?: number;
    domain: string;
    panelConfigName?: string;
    brand?: IDomainBrand;
    meta?: IDomainMeta;
    theme?: IDomainTheme;
    ui?: IDomainUI;
    locale?: IDomainLocale;
    sslEnabled?: boolean;
}
