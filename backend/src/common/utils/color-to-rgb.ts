const NAMED_COLORS: Record<string, string> = {
    cyan: '34, 211, 238',
    teal: '32, 201, 151',
    green: '64, 192, 87',
    lime: '130, 201, 30',
    yellow: '250, 176, 5',
    orange: '253, 126, 20',
    red: '250, 82, 82',
    pink: '230, 73, 128',
    grape: '190, 75, 219',
    violet: '151, 117, 250',
    indigo: '92, 124, 250',
    blue: '34, 139, 230',
    gray: '134, 142, 150',
    dark: '55, 58, 64',
    white: '255, 255, 255',
}

export function colorToRgb(color: string): string {
    const lower = color.toLowerCase().trim()
    if (NAMED_COLORS[lower]) return NAMED_COLORS[lower]

    const hex = lower.replace('#', '')
    if (/^[0-9a-f]{6}$/.test(hex)) {
        const r = parseInt(hex.slice(0, 2), 16)
        const g = parseInt(hex.slice(2, 4), 16)
        const b = parseInt(hex.slice(4, 6), 16)
        return `${r}, ${g}, ${b}`
    }

    return NAMED_COLORS.cyan
}

export const VALID_MANTINE_COLORS = new Set([
    'blue', 'cyan', 'dark', 'grape', 'gray', 'green',
    'indigo', 'lime', 'orange', 'pink', 'red', 'teal', 'violet', 'yellow',
])
