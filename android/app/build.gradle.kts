plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.kotlin.android)
}

android {
    namespace = "net.quakemesh.android"
    compileSdk = 35

    defaultConfig {
        applicationId = "net.quakemesh.android"
        minSdk = 26
        targetSdk = 35
        versionCode = 1
        versionName = "0.0.14"
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
    kotlinOptions {
        jvmTarget = "17"
    }
}

dependencies {
    implementation(libs.androidx.core.ktx)
    implementation(libs.androidx.appcompat)
    implementation(project(":meshsdk"))

    val meshAar = file("libs/meshcore.aar")
    if (meshAar.exists()) {
        implementation(files(meshAar))
    }
}
