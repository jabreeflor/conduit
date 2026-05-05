// Gradle build for the Conduit JetBrains plugin (issue #143).
//
// Targets the IntelliJ Platform 2024.1+ baseline so the same .zip installs in
// IDEA, PyCharm, GoLand, WebStorm, and the rest of the JetBrains family.

plugins {
    id("java")
    id("org.jetbrains.kotlin.jvm") version "1.9.23"
    id("org.jetbrains.intellij") version "1.17.3"
}

group = "ai.conduit"
version = "0.1.0"

repositories { mavenCentral() }

intellij {
    version.set("2024.1")
    type.set("IC") // Community baseline — works on all flavors.
    plugins.set(emptyList())
}

tasks {
    withType<JavaCompile> {
        sourceCompatibility = "17"
        targetCompatibility = "17"
    }
    withType<org.jetbrains.kotlin.gradle.tasks.KotlinCompile> {
        kotlinOptions.jvmTarget = "17"
    }
    patchPluginXml {
        sinceBuild.set("241")
        untilBuild.set("251.*")
    }
    test {
        useJUnitPlatform()
    }
}

dependencies {
    testImplementation("org.junit.jupiter:junit-jupiter:5.10.2")
}
