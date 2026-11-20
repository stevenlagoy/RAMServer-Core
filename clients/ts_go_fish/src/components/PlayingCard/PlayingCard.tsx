import styles from "./PlayingCard.module.scss";

const cardImages = import.meta.glob("../../assets/cards/*.png", {
    eager: true,
    query: "?url",
    import: "default",
}) as Record<string, string>;

const cardWidth = 431, cardHeight = 765

type Props = {
    imgSrc: string,
    name?: string,
    symbol?: string
}

export default function PlayingCard({imgSrc, name, symbol}: Props) {
    const aliasPath = imgSrc.replace(/^@assets\//, "assets/");
    const resolvedSrc = cardImages[`../../${aliasPath}`] ?? imgSrc;

    return (
        <div className={styles.playingCard}>
            <img src={resolvedSrc} alt={symbol ?? ""} title={name} width={cardWidth} height={cardHeight} />
        </div>
    )
}