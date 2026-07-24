<div class="plugin__mobile-header">
    { title }
</div>

<section class="plugin__content">
    <div
        class="plugin__title plugin__title--chevron-back"
        on:click={ () => bcast.emit('rqstOpen', 'menu') }
    >
        { title }
    </div>

    <form class="settings" on:submit|preventDefault={connect}>
        <label class="field">
            <span class="field__label">Bridge IP</span>
            <input
                class:error={!isEndpointValid}
                bind:value={endpoint}
                class="field__input"
                placeholder="127.0.0.1:8787"
                spellcheck="false"
            />
        </label>

        <label class="field">
            <span class="field__label">NMEA sentence</span>
            <select bind:value={sentenceType} class="field__input">
                <option value="GGA">GGA</option>
                <option value="RMC">RMC</option>
            </select>
        </label>

        <div class="actions">
            {#if isConnected}
                <button class="button button--variant-red size-s" type="button" on:click={disconnect}>
                    Disconnect
                </button>
            {:else}
                <button class="button button--variant-orange size-s" disabled={!isEndpointValid} type="submit">
                    Connect
                </button>
            {/if}
        </div>
    </form>

    <div class:online={isConnected} class="connection-state">
        <span class="connection-state__dot"></span>
        <span>{connectionLabel}</span>
    </div>

    <div class="status size-xs">
        {statusMessage}
    </div>

    {#if currentPosition}
        <button class="nmea-row clickable" type="button" on:click={() => focusMarker()}>
            <span class="nmea-row__name">{currentPosition.name}</span>
            <span class="nmea-row__meta">
                {currentPosition.lat.toFixed(6)}, {currentPosition.lon.toFixed(6)}
            </span>
            <span class="nmea-row__meta">
                HDG {formatNumber(currentPosition.heading, 0)} / {formatNumber(currentPosition.speedKnots, 1)} kt
            </span>
            <span class="nmea-row__meta">
                {currentPosition.sentenceType} {currentPosition.fixTime || ''}
            </span>
        </button>
    {/if}

    {#if lastSentence}
        <div class="raw-sentence size-xs">
            {lastSentence}
        </div>
    {/if}
</section>

<script lang="ts">
    import bcast from '@windy/broadcast';
    import { map } from '@windy/map';
    import { onDestroy, onMount } from 'svelte';

    import config from './pluginConfig';
    import { nmeaIcon } from './nmeaIcon';
    import { parseNmeaSentence } from './nmeaParser';
    import type { DisplayedNmeaMarker, NmeaPosition, NmeaSentenceType } from './nmeaTypes';

    const { title } = config;
    const endpointStorageKey = 'windy-nmea-endpoint';
    const sentenceTypeStorageKey = 'windy-nmea-sentence-type';

    let endpoint = '127.0.0.1:8787';
    let sentenceType: NmeaSentenceType = 'RMC';
    let socket: WebSocket | null = null;
    let marker: DisplayedNmeaMarker | null = null;
    let openedPopup: L.Popup | null = null;
    let currentPosition: NmeaPosition | null = null;
    let lastSentence = '';
    let statusMessage = 'Enter the bridge endpoint and connect.';
    let connectionState: 'idle' | 'connecting' | 'connected' | 'error' = 'idle';
    let receiveBuffer = '';

    $: isEndpointValid = validateEndpoint(endpoint);
    $: isConnected = connectionState === 'connected';
    $: connectionLabel = connectionState === 'connecting'
        ? 'Connecting'
        : connectionState === 'connected'
            ? 'Connected'
            : connectionState === 'error'
                ? 'Connection error'
                : 'Disconnected';

    const validateEndpoint = (value: string): boolean => {
        const candidate = value.trim();
        const match = /^(?:wss?:\/\/)?([^:/]+):(\d{1,5})$/.exec(candidate);

        if (!match) {
            return false;
        }

        const host = match[1];
        const port = Number(match[2]);

        if (!Number.isInteger(port) || port < 1 || port > 65535) {
            return false;
        }

        if (host === 'localhost') {
            return true;
        }

        const octets = host.split('.');
        return octets.length === 4 && octets.every(octet => {
            const value = Number(octet);
            return /^\d{1,3}$/.test(octet) && value >= 0 && value <= 255;
        });
    };

    const normalizeEndpoint = (value: string): string => {
        const trimmed = value.trim();
        return /^wss?:\/\//.test(trimmed) ? trimmed : `ws://${trimmed}`;
    };

    const formatNumber = (value: number | null, digits: number): string => (
        value === null ? '--' : value.toFixed(digits)
    );

    const popupContent = (position: NmeaPosition): string => `
        <strong>${position.name}</strong><br />
        ${position.source}<br /><br />
        Lat: ${position.lat.toFixed(6)}<br />
        Lon: ${position.lon.toFixed(6)}<br />
        Heading: ${formatNumber(position.heading, 0)}&deg;<br />
        Speed: ${formatNumber(position.speedKnots, 1)} kt<br />
        Sentence: ${position.sentenceType}
    `;

    const openPopup = (position: NmeaPosition) => {
        openedPopup?.remove();
        openedPopup = new L.Popup({ autoClose: false, closeOnClick: false })
            .setLatLng([position.lat, position.lon])
            .setContent(popupContent(position))
            .openOn(map);
    };

    const updateMarker = (position: NmeaPosition) => {
        currentPosition = position;

        if (!marker) {
            const leafletMarker = new L.Marker([position.lat, position.lon], { icon: nmeaIcon }).addTo(map);
            leafletMarker.on('click', () => {
                if (currentPosition) {
                    openPopup(currentPosition);
                }
            });
            marker = { ...position, marker: leafletMarker };
            map.setView([position.lat, position.lon], Math.max(map.getZoom(), 10));
            return;
        }

        marker.marker.setLatLng([position.lat, position.lon]);
        marker = { ...position, marker: marker.marker };
    };

    const removeMarker = () => {
        openedPopup?.remove();
        openedPopup = null;

        if (marker) {
            map.removeLayer(marker.marker);
            marker = null;
        }
    };

    const focusMarker = () => {
        if (currentPosition) {
            map.setView([currentPosition.lat, currentPosition.lon], Math.max(map.getZoom(), 12));
            openPopup(currentPosition);
        }
    };

    const handleSentence = (sentence: string) => {
        const parsed = parseNmeaSentence(sentence, sentenceType, endpoint);

        if (!parsed) {
            return;
        }

        lastSentence = parsed.rawSentence;
        statusMessage = `Last ${parsed.sentenceType} fix received.`;
        updateMarker(parsed);
    };

    const processIncomingText = (text: string) => {
        receiveBuffer += text;
        const lines = receiveBuffer.split(/\r?\n/);
        receiveBuffer = lines.pop() || '';
        lines.forEach(handleSentence);
    };

    const disconnect = () => {
        socket?.close();
        socket = null;
        connectionState = 'idle';
        statusMessage = 'Disconnected.';
    };

    const connect = () => {
        if (!isEndpointValid) {
            statusMessage = 'Use the format xxx.xxx.xxx.xxx:xxxxx.';
            return;
        }

        disconnect();
        receiveBuffer = '';
        localStorage.setItem(endpointStorageKey, endpoint.trim());
        localStorage.setItem(sentenceTypeStorageKey, sentenceType);

        const url = normalizeEndpoint(endpoint);
        connectionState = 'connecting';
        statusMessage = `Connecting to ${url}`;

        try {
            socket = new WebSocket(url);
        } catch (error) {
            connectionState = 'error';
            statusMessage = error instanceof Error ? error.message : 'Failed to create WebSocket.';
            return;
        }

        socket.onopen = () => {
            connectionState = 'connected';
            statusMessage = `Waiting for ${sentenceType} sentences.`;
        };

        socket.onmessage = event => {
            if (typeof event.data === 'string') {
                processIncomingText(event.data);
            } else if (event.data instanceof Blob) {
                event.data.text().then(processIncomingText).catch(() => {
                    statusMessage = 'Could not read incoming NMEA data.';
                });
            }
        };

        socket.onerror = () => {
            connectionState = 'error';
            statusMessage = 'Connection failed. Check that Windy NMEA Bridge is running.';
        };

        socket.onclose = () => {
            socket = null;
            connectionState = connectionState === 'error' ? 'error' : 'idle';
        };
    };

    export const onopen = () => {
        if (currentPosition) {
            focusMarker();
        }
    };

    onMount(() => {
        endpoint = localStorage.getItem(endpointStorageKey) || endpoint;
        const storedType = localStorage.getItem(sentenceTypeStorageKey);

        if (storedType === 'GGA' || storedType === 'RMC') {
            sentenceType = storedType;
        }
    });

    onDestroy(() => {
        disconnect();
        removeMarker();
    });
</script>

<style lang="less">
    .plugin__content {
        padding-top: 5px;
    }

    .settings {
        margin-bottom: 14px;
    }

    .field {
        display: block;
        margin-bottom: 10px;
    }

    .field__label {
        display: block;
        margin-bottom: 4px;
        color: #52606d;
        font-size: 12px;
        font-weight: 700;
    }

    .field__input {
        width: 100%;
        min-height: 34px;
        padding: 6px 8px;
        border: 1px solid #c7cdd6;
        border-radius: 4px;
        background: #fff;
        color: #1f2933;
        font-size: 13px;
        box-sizing: border-box;
    }

    .field__input.error {
        border-color: #d64545;
    }

    .actions {
        display: flex;
        justify-content: flex-end;
    }

    .connection-state {
        display: flex;
        align-items: center;
        gap: 7px;
        margin-bottom: 8px;
        color: #52606d;
        font-size: 12px;
        font-weight: 700;
    }

    .connection-state__dot {
        width: 8px;
        height: 8px;
        border-radius: 50%;
        background: #9aa5b1;
    }

    .connection-state.online .connection-state__dot {
        background: #2f9e44;
    }

    .status {
        min-height: 18px;
        margin-bottom: 12px;
        color: #6f7782;
    }

    .nmea-row {
        display: block;
        width: 100%;
        margin: 0 0 10px;
        padding: 10px;
        border: 1px solid #d8dde6;
        border-left: 4px solid #0b6bcb;
        border-radius: 4px;
        background: #fff;
        color: #1f2933;
        text-align: left;
    }

    .nmea-row__name,
    .nmea-row__meta {
        display: block;
    }

    .nmea-row__name {
        margin-bottom: 4px;
        font-weight: 700;
    }

    .nmea-row__meta {
        color: #52606d;
        font-size: 12px;
        line-height: 1.4;
    }

    .raw-sentence {
        overflow-wrap: anywhere;
        padding-top: 6px;
        border-top: 1px solid #e4e7eb;
        color: #6f7782;
        line-height: 1.35;
    }
</style>
