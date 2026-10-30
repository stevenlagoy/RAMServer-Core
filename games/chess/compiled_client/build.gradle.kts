plugins {
    java
    application
}

group = "com.ramserver.client"
version = "1.0-SNAPSHOT"

repositories {
    mavenCentral()
}

sourceSets {
    main {
        java {
            srcDirs("src/main/java", "gen")
        }
    }
}

java {
    sourceCompatibility = JavaVersion.VERSION_17
    targetCompatibility = JavaVersion.VERSION_17
}

dependencies {
    implementation("com.google.protobuf:protobuf-java:4.36.2")
    implementation("com.googlecode.lanterna:lanterna:3.1.5")

    testImplementation(platform("org.junit:junit-bom:5.10.0"))
    testImplementation("org.junit.jupiter:junit-jupiter")
}

tasks.named<JavaExec>("run") {
    standardInput = System.`in`
}

application {
    mainClass.set("com.ramserver.client.core.ClientMain")
}

tasks.test {
    useJUnitPlatform()
}