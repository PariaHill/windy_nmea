export const nmeaIcon = L.divIcon({
    html: `<svg viewBox="0 0 32 32" width="32" height="32" xmlns="http://www.w3.org/2000/svg" aria-hidden="true">
            <circle cx="16" cy="16" r="12" fill="#0b6bcb" stroke="#ffffff" stroke-width="3"/>
            <path d="M16 7 L22 22 L16 19 L10 22 Z" fill="#ffffff"/>
        </svg>`,
    iconSize: [32, 32],
    iconAnchor: [16, 16],
    popupAnchor: [0, -16],
    className: 'nmea-marker-icon',
});
