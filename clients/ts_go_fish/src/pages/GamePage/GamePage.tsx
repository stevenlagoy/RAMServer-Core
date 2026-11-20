import styles from "./GamePage.module.scss";
import PlayingCard from "@/PlayingCard";
import { cards } from "@data/cards";

export default function GamePage() {
    return (
        <main className={styles.main}>
            <h1>Go Fish</h1>
            {[...cards].map(([name, card], jdx) => (
                <PlayingCard key={jdx} imgSrc={card.imgSrc} name={name} />
            ))}
        </main>
    )
}