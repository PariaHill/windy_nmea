import type { NmeaPosition, NmeaSentenceType } from './nmeaTypes';

const parseDegrees = (raw: string, hemisphere: string, degreeDigits: number): number | null => {
    if (!raw || !hemisphere) {
        return null;
    }

    const degrees = Number(raw.slice(0, degreeDigits));
    const minutes = Number(raw.slice(degreeDigits));

    if (!Number.isFinite(degrees) || !Number.isFinite(minutes)) {
        return null;
    }

    const sign = hemisphere === 'S' || hemisphere === 'W' ? -1 : 1;
    return sign * (degrees + minutes / 60);
};

const parseNumber = (raw: string): number | null => {
    const value = Number(raw);
    return Number.isFinite(value) ? value : null;
};

const trimChecksum = (sentence: string): string => sentence.split('*')[0];

const getTalkerSentenceType = (field: string): NmeaSentenceType | null => {
    const type = field.slice(-3);
    return type === 'GGA' || type === 'RMC' ? type : null;
};

const isChecksumValid = (sentence: string): boolean => {
    const starIndex = sentence.indexOf('*');

    if (starIndex === -1) {
        return true;
    }

    const expected = sentence.slice(starIndex + 1, starIndex + 3).toUpperCase();
    const payload = sentence.slice(1, starIndex);
    let checksum = 0;

    for (const char of payload) {
        checksum ^= char.charCodeAt(0);
    }

    return checksum.toString(16).toUpperCase().padStart(2, '0') === expected;
};

const parseGga = (fields: string[], source: string, rawSentence: string): NmeaPosition | null => {
    const quality = Number(fields[6]);

    if (!Number.isFinite(quality) || quality <= 0) {
        return null;
    }

    const lat = parseDegrees(fields[2], fields[3], 2);
    const lon = parseDegrees(fields[4], fields[5], 3);

    if (lat === null || lon === null) {
        return null;
    }

    return {
        id: 'trimble-gps',
        name: 'Trimble GPS',
        lat,
        lon,
        heading: null,
        speedKnots: null,
        source,
        sentenceType: 'GGA',
        fixTime: fields[1] || null,
        rawSentence,
    };
};

const parseRmc = (fields: string[], source: string, rawSentence: string): NmeaPosition | null => {
    if (fields[2] !== 'A') {
        return null;
    }

    const lat = parseDegrees(fields[3], fields[4], 2);
    const lon = parseDegrees(fields[5], fields[6], 3);

    if (lat === null || lon === null) {
        return null;
    }

    return {
        id: 'trimble-gps',
        name: 'Trimble GPS',
        lat,
        lon,
        heading: parseNumber(fields[8]),
        speedKnots: parseNumber(fields[7]),
        source,
        sentenceType: 'RMC',
        fixTime: fields[1] || null,
        rawSentence,
    };
};

export const parseNmeaSentence = (
    sentence: string,
    selectedType: NmeaSentenceType,
    source: string
): NmeaPosition | null => {
    const rawSentence = sentence.trim();

    if (!rawSentence.startsWith('$') || !isChecksumValid(rawSentence)) {
        return null;
    }

    const fields = trimChecksum(rawSentence).split(',');
    const sentenceType = getTalkerSentenceType(fields[0]);

    if (sentenceType !== selectedType) {
        return null;
    }

    return sentenceType === 'GGA'
        ? parseGga(fields, source, rawSentence)
        : parseRmc(fields, source, rawSentence);
};
