export type NmeaSentenceType = 'GGA' | 'RMC';

export interface NmeaPosition {
    id: string;
    name: string;
    lat: number;
    lon: number;
    heading: number | null;
    speedKnots: number | null;
    source: string;
    sentenceType: NmeaSentenceType;
    fixTime: string | null;
    rawSentence: string;
}

export interface DisplayedNmeaMarker extends NmeaPosition {
    marker: L.Marker;
}
