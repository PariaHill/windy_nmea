import type { ExternalPluginConfig } from '@windy/interfaces';

const config: ExternalPluginConfig = {
    name: 'windy-plugin-nmea',
    version: '0.1.3',
    icon: '📍',
    title: 'NMEA Markers',
    description: 'Display NMEA position markers on Windy.',
    author: 'PariaHill',
    repository: 'https://github.com/PariaHill/windy_nmea',
    desktopUI: 'rhpane',
    mobileUI: 'small',
    desktopWidth: 260,
    routerPath: '/nmea',
    private: true,
};

export default config;
