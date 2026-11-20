import styles from "./HomePage.module.scss";
import PlayingCard from "@components/PlayingCard";
import { cards } from "@data/cards";

export default function HomePage() {
    return (
        <main className={styles.homeMain}>
            <h1>Go Fish</h1>
            {[...cards].map(([name, card], jdx) => (
                <PlayingCard key={jdx} imgSrc={card.imgSrc} name={name} />
            ))}
        </main>
    )
}